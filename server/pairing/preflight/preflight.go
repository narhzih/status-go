package preflight

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/status-im/status-go/logutils"
	"github.com/status-im/status-go/protocol/common"
	"github.com/status-im/status-go/server"
)

const (
	outboundCheck = "/outbound_check"
	headerPing    = "ping"
	headerPong    = "pong"
)

func preflightHandler(w http.ResponseWriter, r *http.Request) {
	ping := r.Header.Get(headerPing)
	if ping == "" {
		http.Error(w, "no value in 'ping' header", http.StatusBadRequest)
	}

	w.Header().Set(headerPong, ping)
}

func makeCert(address net.IP) (*tls.Certificate, []byte, error) {
	notBefore := time.Now()
	notAfter := notBefore.Add(time.Minute)
	return server.GenerateTLSCert(notBefore, notAfter, address.String())
}

func makeAndStartServer(cert *tls.Certificate, address net.IP) (string, func() error, error) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	waitForPortSet := func(int) {
		wg.Done()
	}

	s := server.NewServer(
		cert,
		address.String(),
		waitForPortSet,
		logutils.ZapLogger().Named("Preflight Server"),
	)

	s.SetHandlers(server.HandlerPatternMap{outboundCheck: preflightHandler})
	err := s.Start()
	if err != nil {
		return "", nil, err
	}

	wg.Wait()
	return s.GetHostname() + ":" + strconv.Itoa(s.GetPort()), s.Stop, nil
}

func makeClient(certPem []byte) (*http.Client, error) {
	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	if ok := rootCAs.AppendCertsFromPEM(certPem); !ok {
		return nil, fmt.Errorf("failed to append certPem to rootCAs")
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: false, // MUST BE FALSE
			RootCAs:            rootCAs,
		},
	}

	return &http.Client{Transport: tr}, nil
}

func makeOutboundCheck(c *http.Client, host string) error {
	u := url.URL{
		Scheme: "https",
		Host:   host,
		Path:   outboundCheck,
	}

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return err
	}

	ping, err := common.RandomAlphanumericString(64)
	if err != nil {
		return err
	}

	req.Header.Set(headerPing, ping)

	resp, err := c.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("response status not ok, received '%d' : '%s'", resp.StatusCode, resp.Status)
	}

	pong := resp.Header.Get(headerPong)
	if pong != ping {
		return fmt.Errorf("ping should match pong: ping '%s', pong '%s'", ping, pong)
	}
	return nil
}

func CheckOutbound() error {
	// cert stuff
	outboundIP, err := server.GetOutboundIP()
	if err != nil {
		return err
	}

	cert, certPem, err := makeCert(outboundIP)
	if err != nil {
		return err
	}

	// server stuff
	host, stop, err := makeAndStartServer(cert, outboundIP)
	if err != nil {
		return err
	}
	defer func() {
		err := stop()
		if err != nil {
			logutils.ZapLogger().Error("error while stopping preflight serve", zap.Error(err))
		}
	}()

	// Client stuff
	c, err := makeClient(certPem)
	if err != nil {
		return err
	}

	return makeOutboundCheck(c, host)
}
