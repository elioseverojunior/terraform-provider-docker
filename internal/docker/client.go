package docker

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/tlsconfig"
)

type ClientConfig struct {
	Host      string
	TLSVerify bool
	CertPath  string
	CACert    string
	Cert      string
	Key       string
}

type Client struct {
	*dockerclient.Client
	config ClientConfig
}

func NewClient(config ClientConfig) (*Client, error) {
	opts := []dockerclient.Opt{
		dockerclient.WithHost(config.Host),
		dockerclient.WithAPIVersionNegotiation(),
	}

	if config.TLSVerify {
		httpClient, err := createTLSHTTPClient(config)
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS HTTP client: %w", err)
		}
		opts = append(opts, dockerclient.WithHTTPClient(httpClient))
	}

	client, err := dockerclient.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Verify connection
	_, err = client.Ping(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker daemon at %s: %w", config.Host, err)
	}

	return &Client{
		Client: client,
		config: config,
	}, nil
}

func createTLSHTTPClient(config ClientConfig) (*http.Client, error) {
	var tlsConfig *tls.Config

	if config.CACert != "" && config.Cert != "" && config.Key != "" {
		// Use certificate content directly
		var err error
		tlsConfig, err = createTLSConfigFromContent(config.CACert, config.Cert, config.Key)
		if err != nil {
			return nil, err
		}
	} else if config.CertPath != "" {
		// Load certificates from path
		options := tlsconfig.Options{
			CAFile:             filepath.Join(config.CertPath, "ca.pem"),
			CertFile:           filepath.Join(config.CertPath, "cert.pem"),
			KeyFile:            filepath.Join(config.CertPath, "key.pem"),
			InsecureSkipVerify: !config.TLSVerify,
		}
		var err error
		tlsConfig, err = tlsconfig.Client(options)
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS config from cert path: %w", err)
		}
	} else {
		return nil, fmt.Errorf("TLS verification enabled but no certificates provided")
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}, nil
}

func createTLSConfigFromContent(caCert, cert, key string) (*tls.Config, error) {
	// Parse CA certificate
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM([]byte(caCert)) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	// Parse client certificate and key
	clientCert, err := tls.X509KeyPair([]byte(cert), []byte(key))
	if err != nil {
		return nil, fmt.Errorf("failed to parse client certificate/key pair: %w", err)
	}

	return &tls.Config{
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{clientCert},
	}, nil
}

func (c *Client) Close() error {
	return c.Client.Close()
}

func LoadCertFromFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read certificate file %s: %w", path, err)
	}
	return string(content), nil
}
