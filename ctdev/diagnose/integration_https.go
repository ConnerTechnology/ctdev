package diagnose

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var errPublicAppliance = errors.New("refusing to send credentials to a public address")

// privateAPIClient confines appliance credentials to the original HTTPS origin.
// System roots authenticate the peer by default. A separately verified leaf
// fingerprint lets a field visit authenticate a self-signed appliance.
func privateAPIClient(endpoint, fingerprint string) (*http.Client, error) {
	origin, err := url.Parse(endpoint)
	if err != nil || origin.Scheme != "https" || origin.Hostname() == "" ||
		origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" {
		return nil, errors.New("appliance endpoint must be an HTTPS URL without credentials, a query, or a fragment")
	}
	if addr, err := netip.ParseAddr(origin.Hostname()); err == nil && !IsPrivate(addr.Unmap()) {
		return nil, errPublicAppliance
	}
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if fingerprint != "" {
		pin, err := hex.DecodeString(strings.ReplaceAll(strings.TrimSpace(fingerprint), ":", ""))
		if err != nil || len(pin) != sha256.Size {
			return nil, errors.New("appliance certificate fingerprint must contain all 32 SHA-256 bytes in hexadecimal")
		}
		// An exact certificate match replaces CA and hostname verification.
		config.InsecureSkipVerify = true
		config.VerifyConnection = func(cs tls.ConnectionState) error {
			sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
			if !bytes.Equal(sum[:], pin) {
				return errors.New("appliance certificate differs from the configured SHA-256 fingerprint")
			}
			return nil
		}
	}
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: config, DialContext: dialPrivate},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			if req.URL.Scheme != origin.Scheme || !strings.EqualFold(req.URL.Host, origin.Host) || req.URL.User != nil {
				return errors.New("refusing redirect outside the configured HTTPS appliance origin")
			}
			return nil
		},
	}, nil
}

var lookupHostIP = net.DefaultResolver.LookupNetIP

// dialPrivate checks every address returned for this connection, then dials
// only those literal addresses. TLS still authenticates the original hostname.
func dialPrivate(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := lookupHostIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("no addresses for appliance")
	}
	for _, ip := range ips {
		if !IsPrivate(ip.Unmap()) {
			return nil, errPublicAppliance
		}
	}
	var dialer net.Dialer
	for _, ip := range ips {
		var conn net.Conn
		conn, err = dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("connect to appliance: %w", err)
}
