// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/juju/cmd"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"launchpad.net/gnuflag"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju/osenv"
	coretesting "github.com/juju/juju/testing"
)

type EnvironmentCommandSuite struct {
	coretesting.FakeJujuHomeSuite
}

var _ = gc.Suite(&EnvironmentCommandSuite{})

func Test(t *testing.T) { gc.TestingT(t) }

func (s *EnvironmentCommandSuite) TestReadCurrentEnvironmentUnset(c *gc.C) {
	env := envcmd.ReadCurrentEnvironment()
	c.Assert(env, gc.Equals, "")
}

func (s *EnvironmentCommandSuite) TestReadCurrentEnvironmentSet(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	env := envcmd.ReadCurrentEnvironment()
	c.Assert(env, gc.Equals, "fubar")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironment(c *gc.C) {
	env, err := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "erewhemos")
	c.Assert(err, gc.IsNil)
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentNothingSet(c *gc.C) {
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	c.Assert(err, gc.IsNil)
	env, err := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "")
	c.Assert(err, gc.IsNil)
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentCurrentEnvironmentSet(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	env, err := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "fubar")
	c.Assert(err, gc.IsNil)
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentJujuEnvSet(c *gc.C) {
	os.Setenv(osenv.JujuEnvEnvKey, "magic")
	env, err := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "magic")
	c.Assert(err, gc.IsNil)
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentBothSet(c *gc.C) {
	os.Setenv(osenv.JujuEnvEnvKey, "magic")
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	env, err := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "magic")
	c.Assert(err, gc.IsNil)
}

func (s *EnvironmentCommandSuite) TestWriteAddsNewline(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	current, err := ioutil.ReadFile(envcmd.GetCurrentEnvironmentFilePath())
	c.Assert(err, gc.IsNil)
	c.Assert(string(current), gc.Equals, "fubar\n")
}

func (*EnvironmentCommandSuite) TestErrorWritingFile(c *gc.C) {
	// Can't write a file over a directory.
	os.MkdirAll(envcmd.GetCurrentEnvironmentFilePath(), 0777)
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.ErrorMatches, "unable to write to the environment file: .*")
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitExplicit(c *gc.C) {
	// Take environment name from command line arg.
	testEnsureEnvName(c, "explicit", "-e", "explicit")
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitMultipleConfigs(c *gc.C) {
	// Take environment name from the default.
	coretesting.WriteEnvironments(c, coretesting.MultipleEnvConfig)
	testEnsureEnvName(c, coretesting.SampleEnvName)
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitSingleConfig(c *gc.C) {
	// Take environment name from the one and only environment,
	// even if it is not explicitly marked as default.
	coretesting.WriteEnvironments(c, coretesting.SingleEnvConfigNoDefault)
	testEnsureEnvName(c, coretesting.SampleEnvName)
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitEnvFile(c *gc.C) {
	// If there is a current-environment file, use that.
	err = envcmd.WriteCurrentEnvironment("fubar")
	testEnsureEnvName(c, "fubar")
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitNoEnvFile(c *gc.C) {
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	c.Assert(err, gc.IsNil)
	testEnsureEnvName(c, "")
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitMultipleConfigNoDefault(c *gc.C) {
	// If there are multiple environments but no default, the connection name is empty.
	coretesting.WriteEnvironments(c, coretesting.MultipleEnvConfigNoDefault)
	testEnsureEnvName(c, "")
}

type testCommand struct {
	envcmd.EnvCommandBase
}

func (c *testCommand) Info() *cmd.Info {
	panic("should not be called")
}

func (c *testCommand) Run(ctx *cmd.Context) error {
	panic("should not be called")
}

// prepareEnvCommand prepares a Command for a call to Init,
// returning the Command and a pointer to a string that will
// contain the environment name after the Command's Init method
// has been called.
func prepareEnvCommand(c *gc.C, name string) (cmd.Command, *string) {
	var flags gnuflag.FlagSet
	var cmd testCommand
	wrapped := envcmd.Wrap(&cmd)
	wrapped.SetFlags(&flags)
	var args []string
	if name != "" {
		args = []string{"-e", name}
	}
	err := flags.Parse(false, args)
	c.Assert(err, gc.IsNil)
	return wrapped, &cmd.EnvName
}

func testEnsureEnvName(c *gc.C, expect string) {
	cmd, envName := prepareEnvCommand(c, "")
	err := cmd.Init(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(*envName, gc.Equals, expect)
}
