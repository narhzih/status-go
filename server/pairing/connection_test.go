package pairing

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/status-im/status-go/server"
	"github.com/status-im/status-go/server/servertest"
)

var (
	connectionString = "cs2:4FHRnp:Q4:uqnnMwVUfJc2Fkcaojet8F1ufKC3hZdGEt47joyBx9yd:BbnZ7Gc66t54a9kEFCf7FW8SGQuYypwHVeNkRYeNoqV6"
)

func TestConnectionParamsSuite(t *testing.T) {
	suite.Run(t, new(ConnectionParamsSuite))
}

type ConnectionParamsSuite struct {
	suite.Suite
	servertest.TestKeyComponents
	servertest.TestCertComponents
	servertest.TestLoggerComponents

	server *BaseServer
}

func (s *ConnectionParamsSuite) SetupSuite() {
	s.SetupKeyComponents(s.T())
	s.SetupCertComponents(s.T())
	s.SetupLoggerComponents()

	cert, _, err := GenerateCertFromKey(s.PK, s.NotBefore, server.DefaultIP.String())
	s.Require().NoError(err)

	bs := server.NewServer(&cert, server.DefaultIP.String(), nil, s.Logger)
	err = bs.SetPort(1337)
	s.Require().NoError(err)

	s.server = &BaseServer{
		Server: bs,
		pk:     &s.PK.PublicKey,
		ek:     s.AES,
	}
}

func (s *ConnectionParamsSuite) TestConnectionParams_ToString() {
	cp, err := s.server.MakeConnectionParams()
	s.Require().NoError(err)

	cps := cp.ToString()
	s.Require().Equal(connectionString, cps)
}

func (s *ConnectionParamsSuite) TestConnectionParams_Generate() {
	cp := new(ConnectionParams)
	err := cp.FromString(connectionString)
	s.Require().NoError(err)

	u, err := cp.URL()
	s.Require().NoError(err)

	s.Require().Equal("https://127.0.0.1:1337", u.String())
	s.Require().Equal(server.DefaultIP.String(), u.Hostname())
	s.Require().Equal("1337", u.Port())

	s.Require().True(cp.publicKey.Equal(&s.PK.PublicKey))
	s.Require().Equal(s.AES, cp.aesKey)
}
