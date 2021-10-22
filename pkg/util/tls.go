package util

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
)

type TLSInputOptions struct {
	Enabled        bool
	ServerCertPath string
	ServerKeyPath  string
	ClientCAPath   string
}

func WrapInputWithTLS(lis net.Listener, options TLSInputOptions) (net.Listener, error) {
	if !options.Enabled {
		return lis, nil
	}

	if options.ServerCertPath == "" || options.ServerKeyPath == "" {
		return nil, fmt.Errorf("when TLS is enabled, both cert and key paths need to be provided")
	}

	cert, err := tls.LoadX509KeyPair(options.ServerCertPath, options.ServerKeyPath)
	if err != nil {
		return nil, fmt.Errorf("could not load X509 key pair: %w", err)
	}

	conf := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	if options.ClientCAPath != "" {
		file, err := os.ReadFile(options.ClientCAPath)
		if err != nil {
			return nil, fmt.Errorf("error while loading client CA: %w", err)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(file) {
			return nil, fmt.Errorf("could not load any client CA certificates")
		}

		conf.ClientCAs = pool
		conf.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return tls.NewListener(lis, conf), nil
}
