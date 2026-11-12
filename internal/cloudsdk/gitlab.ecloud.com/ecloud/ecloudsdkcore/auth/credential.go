package auth

import (
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type CredentialType string

const (
	CredentialAkSk CredentialType = "ECLOUD_AKSK"
	CredentialMop                 = "MOP"
	CredentialS3                  = "ECLOUD_S3"
	CredentialNone                = "NONE"
)

type EncryptionType string

const (
	EncrytMopRsa EncryptionType = "MOP_RSA"
	EncrytNone                  = "NONE"
)

type Credential struct {
	AccessKey      *string
	SecretKey      *string
	SecurityToken  *string
	PrivateKey     *string
	PublicKey      *string
	CredentialType *CredentialType
	EncryptionType *EncryptionType
}

func NewCredential() *Credential {
	return &Credential{
		CredentialType: CredentialTypePointer(CredentialAkSk),
		EncryptionType: EncryptionTypePointer(EncrytNone),
	}
}

func (c *Credential) CopyValues(other *Credential) {
	if utils.IsUnSet(c.AccessKey) && utils.IsSet(other.AccessKey) {
		c.AccessKey = other.AccessKey
	}
	if utils.IsUnSet(c.SecretKey) && utils.IsSet(other.SecretKey) {
		c.SecretKey = other.SecretKey
	}
	if utils.IsUnSet(c.SecurityToken) && utils.IsSet(other.SecurityToken) {
		c.SecurityToken = other.SecurityToken
	}
	if utils.IsUnSet(c.PrivateKey) && utils.IsSet(other.PrivateKey) {
		c.PrivateKey = other.PrivateKey
	}
	if utils.IsUnSet(c.PublicKey) && utils.IsSet(other.PublicKey) {
		c.PublicKey = other.PublicKey
	}
}

func (c *Credential) String() string {
	return utils.Beautify(c)
}

func (c *Credential) GoString() string {
	return c.String()
}

func (c *Credential) ToJsonString() string {
	return utils.ToJsonString(c)
}

type CredentialBuilder struct {
	credential *Credential
}

func NewCredentialBuilder() *CredentialBuilder {
	credential := NewCredential()
	c := &CredentialBuilder{credential: credential}
	return c
}

func (c *CredentialBuilder) AccessKey(accessKey string) *CredentialBuilder {
	c.credential.AccessKey = utils.String(accessKey)
	return c
}

func (c *CredentialBuilder) SecretKey(secretKey string) *CredentialBuilder {
	c.credential.SecretKey = utils.String(secretKey)
	return c
}

func (c *CredentialBuilder) PrivateKey(privateKey string) *CredentialBuilder {
	c.credential.PrivateKey = utils.String(privateKey)
	return c
}

func (c *CredentialBuilder) PublicKey(publicKey string) *CredentialBuilder {
	c.credential.PublicKey = utils.String(publicKey)
	return c
}

func (c *CredentialBuilder) SecurityToken(securityToken string) *CredentialBuilder {
	c.credential.SecurityToken = utils.String(securityToken)
	return c
}

func (c *CredentialBuilder) CredentialType(credentialType CredentialType) *CredentialBuilder {
	c.credential.CredentialType = CredentialTypePointer(credentialType)
	return c
}

func (c *CredentialBuilder) EncryptionType(encryptionType EncryptionType) *CredentialBuilder {
	c.credential.EncryptionType = EncryptionTypePointer(encryptionType)
	return c
}

func (c *CredentialBuilder) Build() *Credential {
	return c.credential
}
