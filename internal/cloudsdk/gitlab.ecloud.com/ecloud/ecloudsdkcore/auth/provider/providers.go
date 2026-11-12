package provider

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/auth"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/errs"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type ICredentialProvider interface {
	GetCredential() (*auth.Credential, error)
}

type BasicCredentialProvider struct {
	credential *auth.Credential
}

func NewBasicCredentialProvider(credential *auth.Credential) *BasicCredentialProvider {
	if utils.IsUnSet(credential) {
		return nil
	}
	if utils.IsUnSet(credential.CredentialType) {
		credential.CredentialType = auth.CredentialTypePointer(auth.CredentialAkSk)
	}
	if utils.IsUnSet(credential.EncryptionType) {
		credential.EncryptionType = auth.EncryptionTypePointer(auth.EncrytNone)
	}
	return &BasicCredentialProvider{
		credential: credential,
	}
}

func (p *BasicCredentialProvider) GetCredential() (*auth.Credential, error) {
	return p.credential, nil
}

func CreateBasicCredentialProvider(credential *auth.Credential) *BasicCredentialProvider {
	return NewBasicCredentialProvider(credential)
}

type EnvCredentialProvider struct{}

func NewEnvCredentialProvider() *EnvCredentialProvider {
	return &EnvCredentialProvider{}
}

func (p *EnvCredentialProvider) GetCredential() (*auth.Credential, error) {
	accessKey := os.Getenv(auth.EnvAkKey)
	secretKey := os.Getenv(auth.EnvSkKey)
	privateKey := os.Getenv(auth.EnvMopPrivateKey)
	publicKey := os.Getenv(auth.EnvMopPublicKey)
	if utils.IsSet(accessKey) && utils.IsSet(secretKey) {
		return auth.NewCredentialBuilder().
			AccessKey(accessKey).
			SecretKey(secretKey).
			PrivateKey(privateKey).
			PublicKey(publicKey).
			Build(), nil
	}
	return nil, errs.NewCredentialError("EnvCredentialProvider: accessKey or secretKey cannot be empty", nil)
}

func CreateEnvCredentialProvider() *EnvCredentialProvider {
	return NewEnvCredentialProvider()
}

type ProfileCredentialProvider struct {
	Credential  *auth.Credential
	profilePath string
}

func NewProfileCredentialProvider(credential *auth.Credential, profilePath string) *ProfileCredentialProvider {
	return &ProfileCredentialProvider{Credential: credential, profilePath: profilePath}
}

func getProfilePath(profilePath string) (string, error) {
	_, err := os.Stat(profilePath)
	if err != nil {
		return "", errs.NewCredentialError("Get the profile directory error", err)
	}
	return profilePath, nil
}

func getProfilePathByDefault() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", errs.NewCredentialError("Get the user home directory error", err)
	}
	dir = filepath.Join(dir, strings.ReplaceAll(auth.PathCredentialFile, "/", string(filepath.Separator)))
	_, err = os.Stat(dir)
	if err != nil {
		return "", errs.NewCredentialError("Get the profile directory error", err)
	}
	return dir, nil
}

func (p *ProfileCredentialProvider) GetCredential() (*auth.Credential, error) {
	path, ok := os.LookupEnv(auth.EnvCredentialFile)
	if !ok {
		var err error
		if p.profilePath != "" {
			path, err = getProfilePath(p.profilePath)
		} else {
			path, err = getProfilePathByDefault()
		}
		if err != nil {
			return nil, err
		}
	}
	if path == "" {
		return nil, errs.NewCredentialError(auth.EnvCredentialFile+" cannot be empty", nil)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, errs.NewCredentialError("Open the profile file error", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	config := make(map[string]string)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			config[key] = value
		}
	}
	return auth.NewCredentialBuilder().
		AccessKey(config[auth.ProfileAccessKey]).
		SecretKey(config[auth.ProfileSecretKey]).
		PrivateKey(config[auth.ProfileMopPrivateKey]).
		PublicKey(config[auth.ProfileMopPublicKey]).
		Build(), nil
}

func CreateProfileCredentialProvider(credential *auth.Credential, profilePath string) *ProfileCredentialProvider {
	return NewProfileCredentialProvider(credential, profilePath)
}

func CreateProfileCredentialProviderByDefault() *ProfileCredentialProvider {
	return NewProfileCredentialProvider(auth.NewCredential(), "")
}

func CreateProfileCredentialProviderByPath(profilePath string) *ProfileCredentialProvider {
	return NewProfileCredentialProvider(auth.NewCredential(), profilePath)
}

type CredentialProviderChain struct {
	providers []ICredentialProvider
}

func NewCredentialProviderChain(providers ...ICredentialProvider) *CredentialProviderChain {
	return &CredentialProviderChain{providers: providers}
}

func (c *CredentialProviderChain) GetCredential() (*auth.Credential, error) {
	for _, provider := range c.providers {
		credential, err := provider.GetCredential()
		if err == nil && credential != nil {
			return credential, nil
		}
	}
	return nil, errs.NewCredentialError("no valid credential found", nil)
}

func CreateCredentialProviderChain(providers ...ICredentialProvider) *CredentialProviderChain {
	return NewCredentialProviderChain(providers...)
}
