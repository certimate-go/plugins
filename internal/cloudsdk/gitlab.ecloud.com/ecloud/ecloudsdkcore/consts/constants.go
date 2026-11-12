package consts

type AuthType string

const (
	AK     AuthType = "AK"
	Token           = "TOKEN"
	NoAuth          = "NO_AUTH"
)

const (
	AccessKey             = "AccessKey"
	Timetamp              = "Timestamp"
	TimestampFormat       = "2006-01-02T15:04:05Z"
	Signature             = "Signature"
	SecretKeyPrefix       = "BC_SIGNATURE&"
	SignatureMethod       = "SignatureMethod"
	SignatureMethodValue  = "HmacSHA256"
	SignatureVersion      = "SignatureVersion"
	SignatureVersionValue = "V2.0"
	SignatureNonce        = "SignatureNonce"
	LineSeparator         = "\n"
	ParameterSeparator    = "&"
	QueryStartSymbol      = "?"
	QuerySeparator        = "="
)
