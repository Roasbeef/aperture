package kirin

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/lightninglabs/kirin/auth"
	"github.com/lightninglabs/kirin/mint"
	"github.com/lightninglabs/kirin/proxy"
	"github.com/lightningnetwork/lnd/build"
	"github.com/lightningnetwork/lnd/cert"
	"github.com/lightningnetwork/lnd/lnrpc"
	"gopkg.in/yaml.v2"
)

const (
	// topLevelKey is the top level key for an etcd cluster where we'll
	// store all LSAT proxy related data.
	topLevelKey = "lsat/proxy"

	// etcdKeyDelimeter is the delimeter we'll use for all etcd keys to
	// represent a path-like structure.
	etcdKeyDelimeter = "/"
)

// Main is the true entrypoint of Kirin.
func Main() {
	// TODO: Prevent from running twice.
	err := start()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// start sets up the proxy server and runs it. This function blocks until a
// shutdown signal is received.
func start() error {
	// First, parse configuration file and set up logging.
	configFile := filepath.Join(kirinDataDir, defaultConfigFilename)
	cfg, err := getConfig(configFile)
	if err != nil {
		return fmt.Errorf("unable to parse config file: %v", err)
	}
	err = setupLogging(cfg)
	if err != nil {
		return fmt.Errorf("unable to set up logging: %v", err)
	}

	// Initialize our etcd client.
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{cfg.Etcd.Host},
		DialTimeout: 5 * time.Second,
		Username:    cfg.Etcd.User,
		Password:    cfg.Etcd.Password,
	})
	if err != nil {
		return fmt.Errorf("unable to connect to etcd: %v", err)
	}

	// Create the proxy and connect it to lnd.
	genInvoiceReq := func() (*lnrpc.Invoice, error) {
		return &lnrpc.Invoice{
			Memo:  "LSAT",
			Value: 1,
		}, nil
	}
	servicesProxy, err := createProxy(cfg, genInvoiceReq, etcdClient)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: http.HandlerFunc(servicesProxy.ServeHTTP),
	}

	// Ensure we create TLS key and certificate if they don't exist.
	tlsKeyFile := filepath.Join(kirinDataDir, defaultTLSKeyFilename)
	tlsCertFile := filepath.Join(kirinDataDir, defaultTLSCertFilename)
	if !fileExists(tlsCertFile) && !fileExists(tlsKeyFile) {
		log.Infof("Generating TLS certificates...")
		err := cert.GenCertPair(
			"kirin autogenerated cert", tlsCertFile, tlsKeyFile,
			nil, nil, cert.DefaultAutogenValidity,
		)
		if err != nil {
			return err
		}
		log.Infof("Done generating TLS certificates")
	}

	// The ListenAndServeTLS below will block until shut down or an error
	// occurs. So we can just defer a cleanup function here that will close
	// everything on shutdown.
	defer cleanup(etcdClient, server)

	// Finally start the server.
	log.Infof("Starting the server, listening on %s.", cfg.ListenAddr)
	return server.ListenAndServeTLS(tlsCertFile, tlsKeyFile)
}

// fileExists reports whether the named file or directory exists.
// This function is taken from https://github.com/btcsuite/btcd
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// getConfig loads and parses the configuration file then checks it for valid
// content.
func getConfig(configFile string) (*config, error) {
	cfg := &config{}
	b, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(b, cfg)
	if err != nil {
		return nil, err
	}

	// Then check the configuration that we got from the config file, all
	// required values need to be set at this point.
	if cfg.ListenAddr == "" {
		return nil, fmt.Errorf("missing listen address for server")
	}
	return cfg, nil
}

// setupLogging parses the debug level and initializes the log file rotator.
func setupLogging(cfg *config) error {
	if cfg.DebugLevel == "" {
		cfg.DebugLevel = defaultLogLevel
	}

	// Now initialize the logger and set the log level.
	logFile := filepath.Join(kirinDataDir, defaultLogFilename)
	err := logWriter.InitLogRotator(
		logFile, defaultMaxLogFileSize, defaultMaxLogFiles,
	)
	if err != nil {
		return err
	}
	return build.ParseAndSetDebugLevels(cfg.DebugLevel, logWriter)
}

// createProxy creates the proxy with all the services it needs.
func createProxy(cfg *config, genInvoiceReq InvoiceRequestGenerator,
	etcdClient *clientv3.Client) (*proxy.Proxy, error) {

	challenger, err := NewLndChallenger(cfg.Authenticator, genInvoiceReq)
	if err != nil {
		return nil, err
	}
	minter := mint.New(&mint.Config{
		Challenger:     challenger,
		Secrets:        newSecretStore(etcdClient),
		ServiceLimiter: newStaticServiceLimiter(cfg.Services),
	})
	authenticator := auth.NewLsatAuthenticator(minter)
	return proxy.New(authenticator, cfg.Services, cfg.StaticRoot)
}

// cleanup closes the given server and shuts down the log rotator.
func cleanup(etcdClient *clientv3.Client, server *http.Server) {
	if err := etcdClient.Close(); err != nil {
		log.Errorf("Error terminating etcd client: %v", err)
	}
	err := server.Close()
	if err != nil {
		log.Errorf("Error closing server: %v", err)
	}
	log.Info("Shutdown complete")
	err = logWriter.Close()
	if err != nil {
		log.Errorf("Could not close log rotator: %v", err)
	}
}