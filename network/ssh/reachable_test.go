// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"net"
	"time"

	_ "github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/network/ssh"
	sshtesting "github.com/juju/juju/network/ssh/testing"
	coretesting "github.com/juju/juju/testing"
)

type SSHReachableHostPortSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&SSHReachableHostPortSuite{})

var searchTimeout = 300 * time.Millisecond
var dialTimeout = 100 * time.Millisecond

func (s *SSHReachableHostPortSuite) TestAllUnreachable(c *gc.C) {
	dialer := &net.Dialer{Timeout: dialTimeout}
	unreachableHPs := closedTCPHostPorts(c, 10)
	best, err := ssh.ReachableHostPort(unreachableHPs, nil, dialer, searchTimeout)
	c.Check(err, gc.ErrorMatches, "cannot connect to any address: .*")
	c.Check(best, gc.Equals, network.HostPort{})
}

func (s *SSHReachableHostPortSuite) TestReachableInvalidPublicKey(c *gc.C) {
	hostPorts := []network.HostPort{
		// We use Key2, but are looking for Pub1
		testSSHServer(c, s, sshtesting.SSHKey2),
	}
	dialer := &net.Dialer{Timeout: dialTimeout}
	best, err := ssh.ReachableHostPort(hostPorts, []string{sshtesting.SSHPub1}, dialer, searchTimeout)
	c.Check(err, gc.ErrorMatches, "cannot connect to any address: .*")
	c.Check(best, gc.Equals, network.HostPort{})
}

func (s *SSHReachableHostPortSuite) TestReachableValidPublicKey(c *gc.C) {
	hostPorts := []network.HostPort{
		testSSHServer(c, s, sshtesting.SSHKey1),
	}
	dialer := &net.Dialer{Timeout: dialTimeout}
	best, err := ssh.ReachableHostPort(hostPorts, []string{sshtesting.SSHPub1}, dialer, searchTimeout)
	c.Check(err, jc.ErrorIsNil)
	c.Check(best, gc.Equals, hostPorts[0])
}

func (s *SSHReachableHostPortSuite) TestReachableMixedPublicKeys(c *gc.C) {
	// One is just closed, one is TCP only, one is SSH but the wrong key, one
	// is SSH with the right key
	fakeHostPort := closedTCPHostPorts(c, 1)[0]
	hostPorts := []network.HostPort{
		fakeHostPort,
		testTCPServer(c, s),
		testSSHServer(c, s, sshtesting.SSHKey2),
		testSSHServer(c, s, sshtesting.SSHKey1),
	}
	dialer := &net.Dialer{Timeout: dialTimeout}
	best, err := ssh.ReachableHostPort(hostPorts, []string{sshtesting.SSHPub1}, dialer, searchTimeout)
	c.Check(best, gc.Equals, network.HostPort{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(best, jc.DeepEquals, hostPorts[3])
}

func (s *SSHReachableHostPortSuite) TestReachableNoPublicKeysPassed(c *gc.C) {
	fakeHostPort := closedTCPHostPorts(c, 1)[0]
	hostPorts := []network.HostPort{
		fakeHostPort,
		testTCPServer(c, s),
	}
	dialer := &net.Dialer{Timeout: dialTimeout}
	best, err := ssh.ReachableHostPort(hostPorts, nil, dialer, searchTimeout)
	c.Check(err, jc.ErrorIsNil)
	c.Check(best, jc.DeepEquals, hostPorts[1]) // the only real listener
}

func (s *SSHReachableHostPortSuite) TestReachableNoPublicKeysAvailable(c *gc.C) {
	fakeHostPort := closedTCPHostPorts(c, 1)[0]
	hostPorts := []network.HostPort{
		fakeHostPort,
		testTCPServer(c, s),
	}
	dialer := &net.Dialer{Timeout: dialTimeout}
	best, err := ssh.ReachableHostPort(hostPorts, []string{sshtesting.SSHPub1}, dialer, searchTimeout)
	c.Check(err, gc.ErrorMatches, "cannot connect to any address: .*")
	c.Check(best, gc.Equals, network.HostPort{})
}

func (s *SSHReachableHostPortSuite) TestMultiplePublicKeys(c *gc.C) {
	hostPorts := []network.HostPort{
		testSSHServer(c, s, sshtesting.SSHKey2),
	}
	dialer := &net.Dialer{Timeout: dialTimeout}
	best, err := ssh.ReachableHostPort(hostPorts, []string{sshtesting.SSHPub1, sshtesting.SSHPub2}, dialer, searchTimeout)
	c.Check(err, jc.ErrorIsNil)
	c.Check(best, gc.Equals, hostPorts[0])
}

// closedTCPHostPorts opens and then immediately closes a bunch of ports and
// saves their port numbers so we're unlikely to find a real listener at that
// address.
func closedTCPHostPorts(c *gc.C, count int) []network.HostPort {
	ports := make([]network.HostPort, count)
	for i := 0; i < count; i++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		c.Assert(err, jc.ErrorIsNil)
		defer listener.Close()
		listenAddress := listener.Addr().String()
		port, err := network.ParseHostPort(listenAddress)
		c.Assert(err, jc.ErrorIsNil)
		ports[i] = *port
	}
	// By the time we return all the listeners are closed
	return ports
}

type Cleaner interface {
	AddCleanup(cleanup func(*gc.C))
}

// testTCPServer only listens on the socket, but doesn't speak SSH
func testTCPServer(c *gc.C, cleaner Cleaner) network.HostPort {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, jc.ErrorIsNil)

	listenAddress := listener.Addr().String()
	hostPort, err := network.ParseHostPort(listenAddress)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("listening on %q", hostPort)

	shutdown := make(chan struct{}, 0)

	go func() {
		for {
			select {
			case <-shutdown:
				// no more listening
				c.Logf("shutting down %s", listenAddress)
				listener.Close()
				return
			default:
			}
			// Don't get hung on Accept, set a deadline
			if tcpListener, ok := listener.(*net.TCPListener); ok {
				tcpListener.SetDeadline(time.Now().Add(1 * time.Second))
			}
			tcpConn, err := listener.Accept()
			if err != nil {
				if netErr, ok := err.(net.Error); ok {
					if netErr.Timeout() {
						// Try again, so we reevaluate if we need to shut down
						continue
					}
				}
			}
			if err != nil {
				c.Logf("failed to accept connection on %s: %v", listenAddress, err)
				continue
			}
			if tcpConn != nil {
				c.Logf("accepted connection on %q from %s", hostPort, tcpConn.RemoteAddr())
				tcpConn.Close()
			}
		}
	}()
	cleaner.AddCleanup(func(*gc.C) { close(shutdown) })

	return *hostPort
}

// testSSHServer will listen on the socket and respond with the appropriate
// public key information and then die.
func testSSHServer(c *gc.C, cleaner Cleaner, privateKey string) network.HostPort {
	address, shutdown := sshtesting.CreateSSHServer(c, privateKey)
	hostPort, err := network.ParseHostPort(address)
	c.Assert(err, jc.ErrorIsNil)
	cleaner.AddCleanup(func(*gc.C) { close(shutdown) })

	return *hostPort
}
