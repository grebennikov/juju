// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/cloud"
	"github.com/juju/testing"
)

type k8sConfigSuite struct {
	testing.IsolationSuite
	dir string
}

var _ = gc.Suite(&k8sConfigSuite{})

var (
	prefixConfigYAML = `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://1.1.1.1:8888
    certificate-authority-data: QQ==
  name: the-cluster
contexts:
- context:
    cluster: the-cluster
    user: the-user
  name: the-context
current-context: the-context
preferences: {}
users:
`
	emptyConfigYAML = `
apiVersion: v1
kind: Config
clusters: []
contexts: []
current-context: ""
preferences: {}
users: []
`

	singleConfigYAML = prefixConfigYAML + `
- name: the-user
  user:
    password: thepassword
    username: theuser
`

	multiConfigYAML = `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://1.1.1.1:8888
    certificate-authority-data: QQ==
  name: the-cluster
- cluster:
    server: https://10.10.10.10:1010
  name: default-cluster
contexts:
- context:
    cluster: the-cluster
    user: the-user
  name: the-context
- context:
    cluster: default-cluster
    user: default-user
  name: default-context
current-context: default-context
preferences: {}
users:
- name: the-user
  user:
    client-certificate-data: QQ==
    client-key-data: Qg==
- name: default-user
  user:
    password: defaultpassword
    username: defaultuser
- name: third-user
  user:
    token: "atoken"
- name: fourth-user
  user:
    client-certificate-data: QQ==
    client-key-data: Qg==
    token: "tokenwithcerttoken"
- name: fifth-user
  user:
    client-certificate-data: QQ==
    client-key-data: Qg==
    username: "fifth-user"
    password: "userpasscertpass"
`
)

func (s *k8sConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.dir = c.MkDir()
}

// writeTempKubeConfig writes yaml to a temp file and sets the
// KUBECONFIG environment variable so that the clientconfig code reads
// it instead of the default.
// The caller must close and remove the returned file.
func (s *k8sConfigSuite) writeTempKubeConfig(c *gc.C, filename string, data string) (*os.File, error) {
	fullpath := filepath.Join(s.dir, filename)
	err := ioutil.WriteFile(fullpath, []byte(data), 0644)
	if err != nil {
		c.Fatal(err.Error())
	}
	os.Setenv("KUBECONFIG", fullpath)

	f, err := os.Open(fullpath)
	return f, err
}

func (s *k8sConfigSuite) TestGetEmptyConfig(c *gc.C) {
	s.assertNewK8sClientConfig(c, newK8sClientConfigTestCase{
		title:              "get empty config",
		configYamlContent:  emptyConfigYAML,
		configYamlFileName: "emptyConfig",
		expected: &clientconfig.ClientConfig{
			Type:           "kubernetes",
			Contexts:       map[string]clientconfig.Context{},
			CurrentContext: "",
			Clouds:         map[string]clientconfig.CloudConfig{},
			Credentials:    map[string]cloud.Credential{},
		}})
}

type newK8sClientConfigTestCase struct {
	title, clusterName, configYamlContent, configYamlFileName string
	expected                                                  *clientconfig.ClientConfig
	errMatch                                                  string
}

func (s *k8sConfigSuite) assertNewK8sClientConfig(c *gc.C, testCase newK8sClientConfigTestCase) {
	f, err := s.writeTempKubeConfig(c, testCase.configYamlFileName, testCase.configYamlContent)
	defer f.Close()
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("test: %s", testCase.title)
	cfg, err := clientconfig.NewK8sClientConfig(f, testCase.clusterName, nil)
	if testCase.errMatch != "" {
		c.Check(err, gc.ErrorMatches, testCase.errMatch)
	} else {
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(cfg, jc.DeepEquals, testCase.expected)
	}
}

func (s *k8sConfigSuite) TestGetSingleConfig(c *gc.C) {
	cred := cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{"username": "theuser", "password": "thepassword"})
	cred.Label = `kubernetes credential "the-user"`
	s.assertNewK8sClientConfig(c, newK8sClientConfigTestCase{
		title:              "assert single config",
		configYamlContent:  singleConfigYAML,
		configYamlFileName: "singleConfig",
		expected: &clientconfig.ClientConfig{
			Type: "kubernetes",
			Contexts: map[string]clientconfig.Context{
				"the-context": {
					CloudName:      "the-cluster",
					CredentialName: "the-user"}},
			CurrentContext: "the-context",
			Clouds: map[string]clientconfig.CloudConfig{
				"the-cluster": {
					Endpoint:   "https://1.1.1.1:8888",
					Attributes: map[string]interface{}{"CAData": "A"}}},
			Credentials: map[string]cloud.Credential{
				"the-user": cred,
			},
		},
	})
}

func (s *k8sConfigSuite) TestConfigErrors(c *gc.C) {
	for _, v := range []newK8sClientConfigTestCase{
		{
			title: "notSupportedAuthProviderTypeConfig",
			configYamlContent: `
- name: the-user
  user:
    auth-provider:
      config:
        cmd-args: config config-helper --format=json
        cmd-path: /usr/lib/google-cloud-sdk/bin/gcloud
        expiry-key: '{.credential.token_expiry}'
        token-key: '{.credential.access_token}'
      name: gcp
`,
			errMatch: `failed to read credentials from kubernetes config: configuration for "the-user" not supported`,
		},
		{
			title: "tokenWithUsernameInvalidConfig",
			configYamlContent: `
- name: the-user
  user:
    password: defaultpassword
    username: defaultuser
    token: nonEmptyToken
`,
			errMatch: `failed to read credentials from kubernetes config: AuthInfo: "the-user" with both Token and User/Pass not valid`,
		},
		{
			title: "emptyClientKeyDataInvalidConfig",
			configYamlContent: `
- name: the-user
  user:
    client-certificate-data: QQ==
    client-key-data:
`,
			errMatch: `failed to read credentials from kubernetes config: empty ClientKeyData for \"the-user\" with auth type \"certificate\" not valid`,
		},
	} {
		v.configYamlFileName = v.title
		v.configYamlContent = prefixConfigYAML + v.configYamlContent
		s.assertNewK8sClientConfig(c, v)
	}
}

func (s *k8sConfigSuite) TestGetMultiConfig(c *gc.C) {
	firstCred := cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{"username": "defaultuser", "password": "defaultpassword"})
	firstCred.Label = `kubernetes credential "default-user"`
	secondCred := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{"ClientCertificateData": "A", "ClientKeyData": "B"})
	secondCred.Label = `kubernetes credential "the-user"`

	for _, v := range []newK8sClientConfigTestCase{
		{
			title:       "no cluster name specified, will select current cluster",
			clusterName: "", // will use current context.
			expected: &clientconfig.ClientConfig{
				Type: "kubernetes",
				Contexts: map[string]clientconfig.Context{
					"default-context": {
						CloudName:      "default-cluster",
						CredentialName: "default-user"},
				},
				CurrentContext: "default-context",
				Clouds: map[string]clientconfig.CloudConfig{
					"default-cluster": {
						Endpoint:   "https://10.10.10.10:1010",
						Attributes: map[string]interface{}{"CAData": ""},
					},
				},
				Credentials: map[string]cloud.Credential{
					"default-user": firstCred,
				},
			},
		},
		{
			title:       "select the-cluster",
			clusterName: "the-cluster",
			expected: &clientconfig.ClientConfig{
				Type: "kubernetes",
				Contexts: map[string]clientconfig.Context{
					"the-context": {
						CloudName:      "the-cluster",
						CredentialName: "the-user"},
				},
				CurrentContext: "default-context",
				Clouds: map[string]clientconfig.CloudConfig{
					"the-cluster": {
						Endpoint:   "https://1.1.1.1:8888",
						Attributes: map[string]interface{}{"CAData": "A"}}},
				Credentials: map[string]cloud.Credential{
					"the-user": secondCred,
				},
			},
		},
		{
			title:       "select default-cluster",
			clusterName: "default-cluster",
			expected: &clientconfig.ClientConfig{
				Type: "kubernetes",
				Contexts: map[string]clientconfig.Context{
					"default-context": {
						CloudName:      "default-cluster",
						CredentialName: "default-user"},
				},
				CurrentContext: "default-context",
				Clouds: map[string]clientconfig.CloudConfig{
					"default-cluster": {
						Endpoint:   "https://10.10.10.10:1010",
						Attributes: map[string]interface{}{"CAData": ""},
					}},
				Credentials: map[string]cloud.Credential{
					"default-user": firstCred,
				},
			},
		},
	} {
		v.configYamlFileName = "multiConfig"
		v.configYamlContent = multiConfigYAML
		s.assertNewK8sClientConfig(c, v)
	}
}

// TestGetSingleConfigReadsFilePaths checks that we handle config
// with certificate/key file paths the same as we do those with
// the data inline.
func (s *k8sConfigSuite) TestGetSingleConfigReadsFilePaths(c *gc.C) {

	singleConfig, err := clientcmd.Load([]byte(singleConfigYAML))
	c.Assert(err, jc.ErrorIsNil)

	tempdir := c.MkDir()
	divert := func(name string, data *[]byte, path *string) {
		*path = filepath.Join(tempdir, name)
		err := ioutil.WriteFile(*path, *data, 0644)
		c.Assert(err, jc.ErrorIsNil)
		*data = nil
	}

	for name, cluster := range singleConfig.Clusters {
		divert(
			"cluster-"+name+".ca",
			&cluster.CertificateAuthorityData,
			&cluster.CertificateAuthority,
		)
	}

	for name, authInfo := range singleConfig.AuthInfos {
		divert(
			"auth-"+name+".cert",
			&authInfo.ClientCertificateData,
			&authInfo.ClientCertificate,
		)
		divert(
			"auth-"+name+".key",
			&authInfo.ClientKeyData,
			&authInfo.ClientKey,
		)
	}

	singleConfigWithPathsYAML, err := clientcmd.Write(*singleConfig)
	c.Assert(err, jc.ErrorIsNil)

	cred := cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{"username": "theuser", "password": "thepassword"})
	cred.Label = `kubernetes credential "the-user"`
	s.assertNewK8sClientConfig(c, newK8sClientConfigTestCase{
		title:              "assert single config",
		configYamlContent:  string(singleConfigWithPathsYAML),
		configYamlFileName: "singleConfigWithPaths",
		expected: &clientconfig.ClientConfig{
			Type: "kubernetes",
			Contexts: map[string]clientconfig.Context{
				"the-context": {
					CloudName:      "the-cluster",
					CredentialName: "the-user"}},
			CurrentContext: "the-context",
			Clouds: map[string]clientconfig.CloudConfig{
				"the-cluster": {
					Endpoint:   "https://1.1.1.1:8888",
					Attributes: map[string]interface{}{"CAData": "A"}}},
			Credentials: map[string]cloud.Credential{
				"the-user": cred,
			},
		},
	})
}
