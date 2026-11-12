package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/consts"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/errs"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/request"
	"github.com/certimate-go/plugins/internal/cloudsdk/gitlab.ecloud.com/ecloud/ecloudsdkcore/utils"
)

type ICredential interface {
	Sign(request *request.HttpRequest, credential *Credential) error
	Encrypt(data []byte, publicKey string) (string, error)
	Decrypt(originStr string, privateKey string) (string, error)
}

type MopCredential struct{}

type NoneCredential struct{}

type AKSKCredential struct{}

func NewMopCredential() *MopCredential {
	return &MopCredential{}
}

func NewAKSKCredential() *AKSKCredential {
	return &AKSKCredential{}
}

func NewNoneCredential() *NoneCredential {
	return &NoneCredential{}
}

var (
	MopCredentialInstance  = NewMopCredential()
	NoneCredentialInstance = NewNoneCredential()
	AKSKCredentialInstance = NewAKSKCredential()
)

func GetCredentialManager(credType CredentialType) ICredential {
	switch credType {
	case CredentialAkSk:
		return AKSKCredentialInstance
	case CredentialMop:
		return MopCredentialInstance
	case CredentialNone:
		return NoneCredentialInstance
	default:
		return AKSKCredentialInstance
	}
}

func (none *NoneCredential) Sign(request *request.HttpRequest, credential *Credential) error {
	request.BuildQueryParamsString()
	return nil
}

func (none *NoneCredential) Encrypt(data []byte, publicKey string) (string, error) {
	return string(data), nil
}

func (none *NoneCredential) Decrypt(originStr string, privateKey string) (string, error) {
	return originStr, nil
}

func (mop *MopCredential) Sign(request *request.HttpRequest, credential *Credential) error {
	if utils.IsUnSet(credential.PrivateKey) {
		return errs.NewInvalidParameterError("RSA private key can not be null", nil)
	}

	request.ConvertQueryParamsFromPath()
	parameters := make(map[string]string)
	for k, v := range request.QueryParams {
		if utils.IsSet(k) {
			parameters[k] = v
		}
	}
	flowdId := utils.Nonce()
	parameters["flowdId"] = flowdId

	keys := make([]string, len(parameters))
	index := 0
	for key := range parameters {
		keys[index] = key
		index++
	}
	sort.Strings(keys)
	builder := strings.Builder{}
	pos := 0
	paramsLen := len(keys)
	for _, key := range keys {
		value := parameters[key]
		builder.WriteString(utils.PercentEncode(key))
		builder.WriteString(consts.QuerySeparator)
		builder.WriteString(utils.PercentEncode(value))
		if pos != paramsLen-1 {
			builder.WriteString(consts.ParameterSeparator)
			pos++
		}
	}
	canonicalQueryString := builder.String()
	keyBytes, err := utils.Base64Decode(*credential.PrivateKey)
	if err != nil {
		return err
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(keyBytes)
	if err != nil {
		return errs.NewCredentialError("ParsePKCS8PrivateKey decode error", err)
	}
	rsaPrivateKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return errs.NewCredentialError("privateKey convert error", nil)
	}
	signature, err := utils.GenerateRSASignature(utils.StringToBytes(canonicalQueryString), rsaPrivateKey)
	if err != nil {
		return errs.NewCredentialError("GenerateRSASignature error", err)
	}
	signStr := utils.Base64Encode(signature)
	request.QueryParams["sign"] = signStr
	request.QueryParams["flowdId"] = flowdId
	GetCredentialManager(CredentialAkSk).Sign(request, credential)
	return nil
}

func (mop *MopCredential) Encrypt(data []byte, publicKey string) (string, error) {
	if utils.IsUnSet(publicKey) {
		return "", errs.NewInvalidParameterError("RSA public key can not be null", nil)
	}
	keyBytes, err := utils.Base64Decode(publicKey)
	if err != nil {
		return "", errs.NewInvalidParameterError("RSA public key is invalid", nil)
	}
	publicKeyInterface, err := x509.ParsePKIXPublicKey(keyBytes)
	if err != nil {
		return "", errs.NewCredentialError("ParsePKIXPublicKey decode error", nil)
	}
	rsaPublicKey, ok := publicKeyInterface.(*rsa.PublicKey)
	if !ok {
		return "", errs.NewCredentialError("publicKey convert error", nil)
	}
	var encryptedData []byte
	for len(data) > 0 {
		var chunk []byte
		if len(data) > 64 {
			chunk, err = rsa.EncryptPKCS1v15(rand.Reader, rsaPublicKey, data[:64])
			data = data[64:]
		} else {
			chunk, err = rsa.EncryptPKCS1v15(rand.Reader, rsaPublicKey, data)
			data = nil
		}
		if err != nil {
			return "", errs.NewCredentialError("RSAPublicKeyEncrypt error", err)
		}
		encryptedData = append(encryptedData, chunk...)
	}
	return utils.Base64Encode(encryptedData), nil
}

func (mop *MopCredential) Decrypt(originStr string, privateKey string) (string, error) {
	if utils.IsUnSet(privateKey) {
		return "", errs.NewInvalidParameterError("RSA private key can not be null", nil)
	}
	data, err := utils.Base64Decode(originStr)
	if err != nil {
		return "", err
	}
	keyBytes, err := utils.Base64Decode(privateKey)
	if err != nil {
		return "", errs.NewInvalidParameterError("RSA private key is invalid", nil)
	}
	privateKeyInterface, err := x509.ParsePKCS8PrivateKey(keyBytes)
	if err != nil {
		return "", errs.NewCredentialError("ParsePKCS8PrivateKey decode error", nil)
	}
	rsaPrivateKey, ok := privateKeyInterface.(*rsa.PrivateKey)
	if !ok {
		return "", errs.NewCredentialError("privateKey convert error", nil)
	}
	var decryptedData []byte
	for len(data) > 0 {
		var chunk []byte
		if len(data) > 75 {
			chunk, err = rsa.DecryptPKCS1v15(rand.Reader, rsaPrivateKey, data[:75])
			data = data[75:]
		} else {
			chunk, err = rsa.DecryptPKCS1v15(rand.Reader, rsaPrivateKey, data)
			data = nil
		}
		if err != nil {
			return "", errs.NewCredentialError("RSAPrivateKeyDecrypt error", err)
		}
		decryptedData = append(decryptedData, chunk...)
	}
	return string(decryptedData), nil
}

func (aksk *AKSKCredential) Sign(request *request.HttpRequest, credential *Credential) error {
	request.ConvertQueryParamsFromPath()
	params := make(map[string]string)
	for key, value := range request.QueryParams {
		params[key] = value
	}
	params[consts.AccessKey] = *credential.AccessKey
	loc, _ := time.LoadLocation("Asia/Shanghai")
	var now time.Time
	if loc != nil {
		now = time.Now().In(loc)
	} else {
		now = time.Now()
	}
	params[consts.Timetamp] = now.Format(consts.TimestampFormat)
	params[consts.SignatureMethod] = consts.SignatureMethodValue
	params[consts.SignatureVersion] = consts.SignatureVersionValue
	params[consts.SignatureNonce] = utils.Nonce()
	keys := make([]string, len(params))
	index := 0
	for key := range params {
		keys[index] = key
		index++
	}
	sort.Strings(keys)
	builder := strings.Builder{}
	pos := 0
	paramsLen := len(keys)
	for _, key := range keys {
		value := params[key]
		builder.WriteString(utils.PercentEncode(key))
		builder.WriteString(consts.QuerySeparator)
		builder.WriteString(utils.PercentEncode(value))
		if pos != paramsLen-1 {
			builder.WriteString(consts.ParameterSeparator)
			pos++
		}
	}
	canonicalQueryString := builder.String()
	hashString := utils.ConvertToHexString(utils.Sha256Encode(canonicalQueryString))
	unescapedPath, err := url.QueryUnescape(request.Path)
	if nil != err {
		return errs.NewSignatureError(err.Error(), err)
	}
	builder.Reset()
	builder.WriteString(strings.ToUpper(request.Method))
	builder.WriteString(consts.LineSeparator)
	builder.WriteString(utils.PercentEncode(unescapedPath))
	builder.WriteString(consts.LineSeparator)
	builder.WriteString(hashString)
	stringToSign := builder.String()
	signature := utils.ConvertToHexString(utils.HmacSha256(stringToSign, consts.SecretKeyPrefix+*credential.SecretKey))
	builder.Reset()
	builder.WriteString(unescapedPath)
	builder.WriteString(consts.QueryStartSymbol)
	builder.WriteString(canonicalQueryString)
	builder.WriteString(consts.ParameterSeparator)
	builder.WriteString(consts.Signature)
	builder.WriteString(consts.QuerySeparator)
	builder.WriteString(utils.PercentEncode(signature))
	request.Path = builder.String()
	return nil
}

func (aksk *AKSKCredential) Encrypt(data []byte, publicKey string) (string, error) {
	return string(data), nil
}

func (aksk *AKSKCredential) Decrypt(originStr string, privateKey string) (string, error) {
	return originStr, nil
}

func CredentialTypePointer(a CredentialType) *CredentialType {
	return &a
}

func EncryptionTypePointer(a EncryptionType) *EncryptionType {
	return &a
}
