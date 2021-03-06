# Lightning Service Authentication Token (LSAT) proxy

Kirin is a HTTP reverse proxy that supports proxying requests for gRPC (HTTP/2)
and REST (HTTP/1 and HTTP/2) backends.

## Installation / Setup

**lnd**

* Make sure lnd ports are reachable.

**kirin**

* Compilation requires go `1.13.x` or later.
* To build `kirin` in the current directory, run `make build` and then copy the
  file `./kirin` from the local directory to the server.
* To build and install `kirin` directly on the machine it will be used, run the
  `make install` command which will place the binary into your `$GOPATH/bin`
  folder.
* Make sure port `8081` is reachable from outside (or whatever port we choose,
  could also be 443 at some point)
* Make sure there is a valid `tls.cert` and `tls.key` file located in the
  `~/.kirin` directory that is valid for the domain that kirin is running on.
  Kirin doesn't support creating its own certificate through Let's Encrypt yet.
  If there is no `tls.cert` and `tls.key` found, a self-signed pair will be
  created.
* Make sure all required configuration items are set in `~/.kirin/kirin.yaml`,
  compare with `sample-conf.yaml`.
* Start kirin without any command line parameters (`./kirin`), all configuration
  is done in the `~/.kirin/kirin.yaml` file.

## Demo

There is a demo installation available at
[test-staging.swap.lightning.today:8081](https://test-staging.swap.lightning.today:8081).

### Use Case 1: Web GUI

If you visit the demo installation in the browser, you see a simple web GUI.
There you can request the current BOS scores for testnet. Notice that you can
only request the scores three times per IP addres. After the free requests have
been used up, you receive an LSAT token/macaroon and are challenged to pay an
invoice to authorize it.

You have two options to pay for the invoice:

1. If you have Joule installed in your browser and connected to a testnet node,
   you can click the "Pay invoice with Joule" button to pay the invoice. After
   successful payment the page should automatically refresh.
1. In case you want to pay the invoice manually, copy the payment request to
   your wallet of choice that has the feature to reveal the preimage after a
   successful payment. Copy the payment preimage in hex format, then click the
   button "Paste preimage of manual payment" and paste it in the dialog box.

### Use Case 2: cURL

First, let's request the BOS scores until we hit the freebie limit:
 
`curl -k -v https://test-staging.swap.lightning.today:8081/availability/v1/btc.json`
 
At some point, we will get an answer 402 with an authorization header:

```
www-authenticate: LSAT macaroon="...", invoice="lntb10n1..."
```

We will need both these values, the `macaroon` and the `invoice` so copy them
to a text file somewhere (without the single quotes!).
Let's pay the invoice now, choose any LN wallet that displays the preimage after
a successful payment. Copy the hex encoded preimage to the text file too once
you get it from the wallet.

Finally, you can issue the authenticated request with the following command:

```
curl -k -v \
--header "Authorization: LSAT <macaroon>:<preimage>" \
https://test-staging.swap.lightning.today:8081/availability/v1/btc.json
```
