//go:build cert

// Copyright 2009 The Go Authors. All rights reserved.
// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log"
	"math/big"
	"net"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli"
)

var Cert = cli.Command{
	Name:  "cert",
	Usage: "Generate self-signed certificate",
	Description: `Generate a self-signed X.509 certificate for a TLS server.
Outputs to 'cert.pem' and 'key.pem' and will overwrite existing files.`,
	Action: runCert,
	Flags: []cli.Flag{
		stringFlag("host", "", "Comma-separated hostnames and IPs to generate a certificate for"),
		stringFlag("ecdsa-curve", "", "ECDSA curve to use to generate a key. Valid values are P224, P256, P384, P521"),
		intFlag("rsa-bits", 2048, "Size of RSA key to generate. Ignored if --ecdsa-curve is set"),
		stringFlag("start-date", "", "Creation date formatted as Jan 1 15:04:05 2011"),
		durationFlag("duration", 365*24*time.Hour, "Duration that certificate is valid for"),
		boolFlag("ca", "whether this cert should be its own Certificate Authority"),
	},
}

func publicKey(priv any) any {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}
}

func pemBlockForKey(priv any) *pem.Block {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			log.Fatalf("Unable to marshal ECDSA private key: %v\n", err)
		}
		return &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}
	default:
		return nil
	}
}

func runCert(ctx *cli.Context) error {
	if len(ctx.String("host")) == 0 {
		log.Fatal("Missing required --host parameter")
	}

	var priv any
	var err error
	switch ctx.String("ecdsa-curve") {
	case "":
		priv, err = rsa.GenerateKey(rand.Reader, ctx.Int("rsa-bits"))
	case "P224":
		priv, err = ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	case "P256":
		priv, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "P384":
		priv, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case "P521":
		priv, err = ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	default:
		log.Fatalf("Unrecognized elliptic curve: %q", ctx.String("ecdsa-curve"))
	}
	if err != nil {
		log.Fatalf("Failed to generate private key: %s", err)
	}

	var notBefore time.Time
	if len(ctx.String("start-date")) == 0 {
		notBefore = time.Now()
	} else {
		notBefore, err = time.Parse("Jan 2 15:04:05 2006", ctx.String("start-date"))
		if err != nil {
			log.Fatalf("Failed to parse creation date: %s", err)
		}
	}

	notAfter := notBefore.Add(ctx.Duration("duration"))

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		log.Fatalf("Failed to generate serial number: %s", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
			CommonName:   "Gogs",
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	hosts := strings.Split(ctx.String("host"), ",")
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	if ctx.Bool("ca") {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey(priv), priv)
	if err != nil {
		log.Fatalf("Failed to create certificate: %s", err)
	}

	certOut, err := os.Create("cert.pem")
	if err != nil {
		log.Fatalf("Failed to open cert.pem for writing: %s", err)
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()
	log.Println("Written cert.pem")

	keyOut, err := os.OpenFile("key.pem", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Failed to open key.pem for writing: %v\n", err)
	}
	pem.Encode(keyOut, pemBlockForKey(priv))
	keyOut.Close()
	log.Println("Written key.pem")

	return nil
}
