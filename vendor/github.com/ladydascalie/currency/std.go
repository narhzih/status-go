package currency

/*-------------------------------+
| Code generated by std_currency |
|          DO NOT EDIT           |
+-------------------------------*/

import "fmt"

// Currency defines a currency containing
// It's code, taken from the constants above
// as well as it's minor units, as an integer.
type Currency struct {
	code       string
	minorUnits int
	factor     int
}

// Code returns the currency code to the user
func (c *Currency) Code() string { return c.code }

// MinorUnits returns the minor unit to the user
func (c *Currency) MinorUnits() int { return c.minorUnits }

// Factor returns the factor by which a float should be multiplied
// to get back to it's smallest denomination
//
// Example:
//  pence := 100.00 * currency.GBP.Factor()
func (c *Currency) Factor() int { return c.factor }

// FactorI64 returns the factor, converted to a int64
func (c *Currency) FactorI64() int64 { return int64(c.factor) }

// FactorF64 returns the factor, converted to a float64
func (c *Currency) FactorF64() float64 { return float64(c.factor) }

// Get returns a currency struct if the provided
// code is contained within the valid codes. Otherwise
// an error will be returned
func Get(code string) (*Currency, error) {
	if Valid(code) {
		val, ok := currencies[code]
		if ok {
			return &val, nil
		}
	}
	return nil, fmt.Errorf("currency: could not find currency with code: %q", code)
}

// Valid checks if a provided code is contained
// inside the provided ValidCodes slice
func Valid(code string) bool {
	for _, c := range ValidCodes {
		if c == code {
			return true
		}
	}
	return false
}

// Following are all the structs containing currency data
var (
	// AED currency struct
	AED = Currency{code: "AED", minorUnits: 2, factor: 100}
	// AFN currency struct
	AFN = Currency{code: "AFN", minorUnits: 2, factor: 100}
	// ALL currency struct
	ALL = Currency{code: "ALL", minorUnits: 2, factor: 100}
	// AMD currency struct
	AMD = Currency{code: "AMD", minorUnits: 2, factor: 100}
	// ANG currency struct
	ANG = Currency{code: "ANG", minorUnits: 2, factor: 100}
	// AOA currency struct
	AOA = Currency{code: "AOA", minorUnits: 2, factor: 100}
	// ARS currency struct
	ARS = Currency{code: "ARS", minorUnits: 2, factor: 100}
	// AUD currency struct
	AUD = Currency{code: "AUD", minorUnits: 2, factor: 100}
	// AWG currency struct
	AWG = Currency{code: "AWG", minorUnits: 2, factor: 100}
	// AZN currency struct
	AZN = Currency{code: "AZN", minorUnits: 2, factor: 100}
	// BAM currency struct
	BAM = Currency{code: "BAM", minorUnits: 2, factor: 100}
	// BBD currency struct
	BBD = Currency{code: "BBD", minorUnits: 2, factor: 100}
	// BDT currency struct
	BDT = Currency{code: "BDT", minorUnits: 2, factor: 100}
	// BGN currency struct
	BGN = Currency{code: "BGN", minorUnits: 2, factor: 100}
	// BHD currency struct
	BHD = Currency{code: "BHD", minorUnits: 3, factor: 1000}
	// BIF currency struct
	BIF = Currency{code: "BIF", minorUnits: 0, factor: 1}
	// BMD currency struct
	BMD = Currency{code: "BMD", minorUnits: 2, factor: 100}
	// BND currency struct
	BND = Currency{code: "BND", minorUnits: 2, factor: 100}
	// BOB currency struct
	BOB = Currency{code: "BOB", minorUnits: 2, factor: 100}
	// BOV currency struct
	BOV = Currency{code: "BOV", minorUnits: 2, factor: 100}
	// BRL currency struct
	BRL = Currency{code: "BRL", minorUnits: 2, factor: 100}
	// BSD currency struct
	BSD = Currency{code: "BSD", minorUnits: 2, factor: 100}
	// BTN currency struct
	BTN = Currency{code: "BTN", minorUnits: 2, factor: 100}
	// BWP currency struct
	BWP = Currency{code: "BWP", minorUnits: 2, factor: 100}
	// BYN currency struct
	BYN = Currency{code: "BYN", minorUnits: 2, factor: 100}
	// BZD currency struct
	BZD = Currency{code: "BZD", minorUnits: 2, factor: 100}
	// CAD currency struct
	CAD = Currency{code: "CAD", minorUnits: 2, factor: 100}
	// CDF currency struct
	CDF = Currency{code: "CDF", minorUnits: 2, factor: 100}
	// CHE currency struct
	CHE = Currency{code: "CHE", minorUnits: 2, factor: 100}
	// CHF currency struct
	CHF = Currency{code: "CHF", minorUnits: 2, factor: 100}
	// CHW currency struct
	CHW = Currency{code: "CHW", minorUnits: 2, factor: 100}
	// CLF currency struct
	CLF = Currency{code: "CLF", minorUnits: 4, factor: 10000}
	// CLP currency struct
	CLP = Currency{code: "CLP", minorUnits: 0, factor: 1}
	// CNY currency struct
	CNY = Currency{code: "CNY", minorUnits: 2, factor: 100}
	// COP currency struct
	COP = Currency{code: "COP", minorUnits: 2, factor: 100}
	// COU currency struct
	COU = Currency{code: "COU", minorUnits: 2, factor: 100}
	// CRC currency struct
	CRC = Currency{code: "CRC", minorUnits: 2, factor: 100}
	// CUC currency struct
	CUC = Currency{code: "CUC", minorUnits: 2, factor: 100}
	// CUP currency struct
	CUP = Currency{code: "CUP", minorUnits: 2, factor: 100}
	// CVE currency struct
	CVE = Currency{code: "CVE", minorUnits: 2, factor: 100}
	// CZK currency struct
	CZK = Currency{code: "CZK", minorUnits: 2, factor: 100}
	// DJF currency struct
	DJF = Currency{code: "DJF", minorUnits: 0, factor: 1}
	// DKK currency struct
	DKK = Currency{code: "DKK", minorUnits: 2, factor: 100}
	// DOP currency struct
	DOP = Currency{code: "DOP", minorUnits: 2, factor: 100}
	// DZD currency struct
	DZD = Currency{code: "DZD", minorUnits: 2, factor: 100}
	// EGP currency struct
	EGP = Currency{code: "EGP", minorUnits: 2, factor: 100}
	// ERN currency struct
	ERN = Currency{code: "ERN", minorUnits: 2, factor: 100}
	// ETB currency struct
	ETB = Currency{code: "ETB", minorUnits: 2, factor: 100}
	// EUR currency struct
	EUR = Currency{code: "EUR", minorUnits: 2, factor: 100}
	// FJD currency struct
	FJD = Currency{code: "FJD", minorUnits: 2, factor: 100}
	// FKP currency struct
	FKP = Currency{code: "FKP", minorUnits: 2, factor: 100}
	// GBP currency struct
	GBP = Currency{code: "GBP", minorUnits: 2, factor: 100}
	// GEL currency struct
	GEL = Currency{code: "GEL", minorUnits: 2, factor: 100}
	// GHS currency struct
	GHS = Currency{code: "GHS", minorUnits: 2, factor: 100}
	// GIP currency struct
	GIP = Currency{code: "GIP", minorUnits: 2, factor: 100}
	// GMD currency struct
	GMD = Currency{code: "GMD", minorUnits: 2, factor: 100}
	// GNF currency struct
	GNF = Currency{code: "GNF", minorUnits: 0, factor: 1}
	// GTQ currency struct
	GTQ = Currency{code: "GTQ", minorUnits: 2, factor: 100}
	// GYD currency struct
	GYD = Currency{code: "GYD", minorUnits: 2, factor: 100}
	// HKD currency struct
	HKD = Currency{code: "HKD", minorUnits: 2, factor: 100}
	// HNL currency struct
	HNL = Currency{code: "HNL", minorUnits: 2, factor: 100}
	// HTG currency struct
	HTG = Currency{code: "HTG", minorUnits: 2, factor: 100}
	// HUF currency struct
	HUF = Currency{code: "HUF", minorUnits: 2, factor: 100}
	// IDR currency struct
	IDR = Currency{code: "IDR", minorUnits: 2, factor: 100}
	// ILS currency struct
	ILS = Currency{code: "ILS", minorUnits: 2, factor: 100}
	// INR currency struct
	INR = Currency{code: "INR", minorUnits: 2, factor: 100}
	// IQD currency struct
	IQD = Currency{code: "IQD", minorUnits: 3, factor: 1000}
	// IRR currency struct
	IRR = Currency{code: "IRR", minorUnits: 2, factor: 100}
	// ISK currency struct
	ISK = Currency{code: "ISK", minorUnits: 0, factor: 1}
	// JMD currency struct
	JMD = Currency{code: "JMD", minorUnits: 2, factor: 100}
	// JOD currency struct
	JOD = Currency{code: "JOD", minorUnits: 3, factor: 1000}
	// JPY currency struct
	JPY = Currency{code: "JPY", minorUnits: 0, factor: 1}
	// KES currency struct
	KES = Currency{code: "KES", minorUnits: 2, factor: 100}
	// KGS currency struct
	KGS = Currency{code: "KGS", minorUnits: 2, factor: 100}
	// KHR currency struct
	KHR = Currency{code: "KHR", minorUnits: 2, factor: 100}
	// KMF currency struct
	KMF = Currency{code: "KMF", minorUnits: 0, factor: 1}
	// KPW currency struct
	KPW = Currency{code: "KPW", minorUnits: 2, factor: 100}
	// KRW currency struct
	KRW = Currency{code: "KRW", minorUnits: 0, factor: 1}
	// KWD currency struct
	KWD = Currency{code: "KWD", minorUnits: 3, factor: 1000}
	// KYD currency struct
	KYD = Currency{code: "KYD", minorUnits: 2, factor: 100}
	// KZT currency struct
	KZT = Currency{code: "KZT", minorUnits: 2, factor: 100}
	// LAK currency struct
	LAK = Currency{code: "LAK", minorUnits: 2, factor: 100}
	// LBP currency struct
	LBP = Currency{code: "LBP", minorUnits: 2, factor: 100}
	// LKR currency struct
	LKR = Currency{code: "LKR", minorUnits: 2, factor: 100}
	// LRD currency struct
	LRD = Currency{code: "LRD", minorUnits: 2, factor: 100}
	// LSL currency struct
	LSL = Currency{code: "LSL", minorUnits: 2, factor: 100}
	// LYD currency struct
	LYD = Currency{code: "LYD", minorUnits: 3, factor: 1000}
	// MAD currency struct
	MAD = Currency{code: "MAD", minorUnits: 2, factor: 100}
	// MDL currency struct
	MDL = Currency{code: "MDL", minorUnits: 2, factor: 100}
	// MGA currency struct
	MGA = Currency{code: "MGA", minorUnits: 2, factor: 100}
	// MKD currency struct
	MKD = Currency{code: "MKD", minorUnits: 2, factor: 100}
	// MMK currency struct
	MMK = Currency{code: "MMK", minorUnits: 2, factor: 100}
	// MNT currency struct
	MNT = Currency{code: "MNT", minorUnits: 2, factor: 100}
	// MOP currency struct
	MOP = Currency{code: "MOP", minorUnits: 2, factor: 100}
	// MRU currency struct
	MRU = Currency{code: "MRU", minorUnits: 2, factor: 100}
	// MUR currency struct
	MUR = Currency{code: "MUR", minorUnits: 2, factor: 100}
	// MVR currency struct
	MVR = Currency{code: "MVR", minorUnits: 2, factor: 100}
	// MWK currency struct
	MWK = Currency{code: "MWK", minorUnits: 2, factor: 100}
	// MXN currency struct
	MXN = Currency{code: "MXN", minorUnits: 2, factor: 100}
	// MXV currency struct
	MXV = Currency{code: "MXV", minorUnits: 2, factor: 100}
	// MYR currency struct
	MYR = Currency{code: "MYR", minorUnits: 2, factor: 100}
	// MZN currency struct
	MZN = Currency{code: "MZN", minorUnits: 2, factor: 100}
	// NAD currency struct
	NAD = Currency{code: "NAD", minorUnits: 2, factor: 100}
	// NGN currency struct
	NGN = Currency{code: "NGN", minorUnits: 2, factor: 100}
	// NIO currency struct
	NIO = Currency{code: "NIO", minorUnits: 2, factor: 100}
	// NOK currency struct
	NOK = Currency{code: "NOK", minorUnits: 2, factor: 100}
	// NPR currency struct
	NPR = Currency{code: "NPR", minorUnits: 2, factor: 100}
	// NZD currency struct
	NZD = Currency{code: "NZD", minorUnits: 2, factor: 100}
	// OMR currency struct
	OMR = Currency{code: "OMR", minorUnits: 3, factor: 1000}
	// PAB currency struct
	PAB = Currency{code: "PAB", minorUnits: 2, factor: 100}
	// PEN currency struct
	PEN = Currency{code: "PEN", minorUnits: 2, factor: 100}
	// PGK currency struct
	PGK = Currency{code: "PGK", minorUnits: 2, factor: 100}
	// PHP currency struct
	PHP = Currency{code: "PHP", minorUnits: 2, factor: 100}
	// PKR currency struct
	PKR = Currency{code: "PKR", minorUnits: 2, factor: 100}
	// PLN currency struct
	PLN = Currency{code: "PLN", minorUnits: 2, factor: 100}
	// PYG currency struct
	PYG = Currency{code: "PYG", minorUnits: 0, factor: 1}
	// QAR currency struct
	QAR = Currency{code: "QAR", minorUnits: 2, factor: 100}
	// RON currency struct
	RON = Currency{code: "RON", minorUnits: 2, factor: 100}
	// RSD currency struct
	RSD = Currency{code: "RSD", minorUnits: 2, factor: 100}
	// RUB currency struct
	RUB = Currency{code: "RUB", minorUnits: 2, factor: 100}
	// RWF currency struct
	RWF = Currency{code: "RWF", minorUnits: 0, factor: 1}
	// SAR currency struct
	SAR = Currency{code: "SAR", minorUnits: 2, factor: 100}
	// SBD currency struct
	SBD = Currency{code: "SBD", minorUnits: 2, factor: 100}
	// SCR currency struct
	SCR = Currency{code: "SCR", minorUnits: 2, factor: 100}
	// SDG currency struct
	SDG = Currency{code: "SDG", minorUnits: 2, factor: 100}
	// SEK currency struct
	SEK = Currency{code: "SEK", minorUnits: 2, factor: 100}
	// SGD currency struct
	SGD = Currency{code: "SGD", minorUnits: 2, factor: 100}
	// SHP currency struct
	SHP = Currency{code: "SHP", minorUnits: 2, factor: 100}
	// SLE currency struct
	SLE = Currency{code: "SLE", minorUnits: 2, factor: 100}
	// SLL currency struct
	SLL = Currency{code: "SLL", minorUnits: 2, factor: 100}
	// SOS currency struct
	SOS = Currency{code: "SOS", minorUnits: 2, factor: 100}
	// SRD currency struct
	SRD = Currency{code: "SRD", minorUnits: 2, factor: 100}
	// SSP currency struct
	SSP = Currency{code: "SSP", minorUnits: 2, factor: 100}
	// STN currency struct
	STN = Currency{code: "STN", minorUnits: 2, factor: 100}
	// SVC currency struct
	SVC = Currency{code: "SVC", minorUnits: 2, factor: 100}
	// SYP currency struct
	SYP = Currency{code: "SYP", minorUnits: 2, factor: 100}
	// SZL currency struct
	SZL = Currency{code: "SZL", minorUnits: 2, factor: 100}
	// THB currency struct
	THB = Currency{code: "THB", minorUnits: 2, factor: 100}
	// TJS currency struct
	TJS = Currency{code: "TJS", minorUnits: 2, factor: 100}
	// TMT currency struct
	TMT = Currency{code: "TMT", minorUnits: 2, factor: 100}
	// TND currency struct
	TND = Currency{code: "TND", minorUnits: 3, factor: 1000}
	// TOP currency struct
	TOP = Currency{code: "TOP", minorUnits: 2, factor: 100}
	// TRY currency struct
	TRY = Currency{code: "TRY", minorUnits: 2, factor: 100}
	// TTD currency struct
	TTD = Currency{code: "TTD", minorUnits: 2, factor: 100}
	// TWD currency struct
	TWD = Currency{code: "TWD", minorUnits: 2, factor: 100}
	// TZS currency struct
	TZS = Currency{code: "TZS", minorUnits: 2, factor: 100}
	// UAH currency struct
	UAH = Currency{code: "UAH", minorUnits: 2, factor: 100}
	// UGX currency struct
	UGX = Currency{code: "UGX", minorUnits: 0, factor: 1}
	// USD currency struct
	USD = Currency{code: "USD", minorUnits: 2, factor: 100}
	// USN currency struct
	USN = Currency{code: "USN", minorUnits: 2, factor: 100}
	// UYI currency struct
	UYI = Currency{code: "UYI", minorUnits: 0, factor: 1}
	// UYU currency struct
	UYU = Currency{code: "UYU", minorUnits: 2, factor: 100}
	// UYW currency struct
	UYW = Currency{code: "UYW", minorUnits: 4, factor: 10000}
	// UZS currency struct
	UZS = Currency{code: "UZS", minorUnits: 2, factor: 100}
	// VED currency struct
	VED = Currency{code: "VED", minorUnits: 2, factor: 100}
	// VES currency struct
	VES = Currency{code: "VES", minorUnits: 2, factor: 100}
	// VND currency struct
	VND = Currency{code: "VND", minorUnits: 0, factor: 1}
	// VUV currency struct
	VUV = Currency{code: "VUV", minorUnits: 0, factor: 1}
	// WST currency struct
	WST = Currency{code: "WST", minorUnits: 2, factor: 100}
	// XAF currency struct
	XAF = Currency{code: "XAF", minorUnits: 0, factor: 1}
	// XAG currency struct
	XAG = Currency{code: "XAG", minorUnits: 0, factor: 1}
	// XAU currency struct
	XAU = Currency{code: "XAU", minorUnits: 0, factor: 1}
	// XBA currency struct
	XBA = Currency{code: "XBA", minorUnits: 0, factor: 1}
	// XBB currency struct
	XBB = Currency{code: "XBB", minorUnits: 0, factor: 1}
	// XBC currency struct
	XBC = Currency{code: "XBC", minorUnits: 0, factor: 1}
	// XBD currency struct
	XBD = Currency{code: "XBD", minorUnits: 0, factor: 1}
	// XCD currency struct
	XCD = Currency{code: "XCD", minorUnits: 2, factor: 100}
	// XDR currency struct
	XDR = Currency{code: "XDR", minorUnits: 0, factor: 1}
	// XOF currency struct
	XOF = Currency{code: "XOF", minorUnits: 0, factor: 1}
	// XPD currency struct
	XPD = Currency{code: "XPD", minorUnits: 0, factor: 1}
	// XPF currency struct
	XPF = Currency{code: "XPF", minorUnits: 0, factor: 1}
	// XPT currency struct
	XPT = Currency{code: "XPT", minorUnits: 0, factor: 1}
	// XSU currency struct
	XSU = Currency{code: "XSU", minorUnits: 0, factor: 1}
	// XTS currency struct
	XTS = Currency{code: "XTS", minorUnits: 0, factor: 1}
	// XUA currency struct
	XUA = Currency{code: "XUA", minorUnits: 0, factor: 1}
	// XXX currency struct
	XXX = Currency{code: "XXX", minorUnits: 0, factor: 1}
	// YER currency struct
	YER = Currency{code: "YER", minorUnits: 2, factor: 100}
	// ZAR currency struct
	ZAR = Currency{code: "ZAR", minorUnits: 2, factor: 100}
	// ZMW currency struct
	ZMW = Currency{code: "ZMW", minorUnits: 2, factor: 100}
	// ZWL currency struct
	ZWL = Currency{code: "ZWL", minorUnits: 2, factor: 100}
)

var currencies = map[string]Currency{
	"AED": AED,
	"AFN": AFN,
	"ALL": ALL,
	"AMD": AMD,
	"ANG": ANG,
	"AOA": AOA,
	"ARS": ARS,
	"AUD": AUD,
	"AWG": AWG,
	"AZN": AZN,
	"BAM": BAM,
	"BBD": BBD,
	"BDT": BDT,
	"BGN": BGN,
	"BHD": BHD,
	"BIF": BIF,
	"BMD": BMD,
	"BND": BND,
	"BOB": BOB,
	"BOV": BOV,
	"BRL": BRL,
	"BSD": BSD,
	"BTN": BTN,
	"BWP": BWP,
	"BYN": BYN,
	"BZD": BZD,
	"CAD": CAD,
	"CDF": CDF,
	"CHE": CHE,
	"CHF": CHF,
	"CHW": CHW,
	"CLF": CLF,
	"CLP": CLP,
	"CNY": CNY,
	"COP": COP,
	"COU": COU,
	"CRC": CRC,
	"CUC": CUC,
	"CUP": CUP,
	"CVE": CVE,
	"CZK": CZK,
	"DJF": DJF,
	"DKK": DKK,
	"DOP": DOP,
	"DZD": DZD,
	"EGP": EGP,
	"ERN": ERN,
	"ETB": ETB,
	"EUR": EUR,
	"FJD": FJD,
	"FKP": FKP,
	"GBP": GBP,
	"GEL": GEL,
	"GHS": GHS,
	"GIP": GIP,
	"GMD": GMD,
	"GNF": GNF,
	"GTQ": GTQ,
	"GYD": GYD,
	"HKD": HKD,
	"HNL": HNL,
	"HTG": HTG,
	"HUF": HUF,
	"IDR": IDR,
	"ILS": ILS,
	"INR": INR,
	"IQD": IQD,
	"IRR": IRR,
	"ISK": ISK,
	"JMD": JMD,
	"JOD": JOD,
	"JPY": JPY,
	"KES": KES,
	"KGS": KGS,
	"KHR": KHR,
	"KMF": KMF,
	"KPW": KPW,
	"KRW": KRW,
	"KWD": KWD,
	"KYD": KYD,
	"KZT": KZT,
	"LAK": LAK,
	"LBP": LBP,
	"LKR": LKR,
	"LRD": LRD,
	"LSL": LSL,
	"LYD": LYD,
	"MAD": MAD,
	"MDL": MDL,
	"MGA": MGA,
	"MKD": MKD,
	"MMK": MMK,
	"MNT": MNT,
	"MOP": MOP,
	"MRU": MRU,
	"MUR": MUR,
	"MVR": MVR,
	"MWK": MWK,
	"MXN": MXN,
	"MXV": MXV,
	"MYR": MYR,
	"MZN": MZN,
	"NAD": NAD,
	"NGN": NGN,
	"NIO": NIO,
	"NOK": NOK,
	"NPR": NPR,
	"NZD": NZD,
	"OMR": OMR,
	"PAB": PAB,
	"PEN": PEN,
	"PGK": PGK,
	"PHP": PHP,
	"PKR": PKR,
	"PLN": PLN,
	"PYG": PYG,
	"QAR": QAR,
	"RON": RON,
	"RSD": RSD,
	"RUB": RUB,
	"RWF": RWF,
	"SAR": SAR,
	"SBD": SBD,
	"SCR": SCR,
	"SDG": SDG,
	"SEK": SEK,
	"SGD": SGD,
	"SHP": SHP,
	"SLE": SLE,
	"SLL": SLL,
	"SOS": SOS,
	"SRD": SRD,
	"SSP": SSP,
	"STN": STN,
	"SVC": SVC,
	"SYP": SYP,
	"SZL": SZL,
	"THB": THB,
	"TJS": TJS,
	"TMT": TMT,
	"TND": TND,
	"TOP": TOP,
	"TRY": TRY,
	"TTD": TTD,
	"TWD": TWD,
	"TZS": TZS,
	"UAH": UAH,
	"UGX": UGX,
	"USD": USD,
	"USN": USN,
	"UYI": UYI,
	"UYU": UYU,
	"UYW": UYW,
	"UZS": UZS,
	"VED": VED,
	"VES": VES,
	"VND": VND,
	"VUV": VUV,
	"WST": WST,
	"XAF": XAF,
	"XAG": XAG,
	"XAU": XAU,
	"XBA": XBA,
	"XBB": XBB,
	"XBC": XBC,
	"XBD": XBD,
	"XCD": XCD,
	"XDR": XDR,
	"XOF": XOF,
	"XPD": XPD,
	"XPF": XPF,
	"XPT": XPT,
	"XSU": XSU,
	"XTS": XTS,
	"XUA": XUA,
	"XXX": XXX,
	"YER": YER,
	"ZAR": ZAR,
	"ZMW": ZMW,
	"ZWL": ZWL,
}

// ValidCodes is provided so that you may build your own validation against it
var ValidCodes = []string{
	"AED",
	"AFN",
	"ALL",
	"AMD",
	"ANG",
	"AOA",
	"ARS",
	"AUD",
	"AWG",
	"AZN",
	"BAM",
	"BBD",
	"BDT",
	"BGN",
	"BHD",
	"BIF",
	"BMD",
	"BND",
	"BOB",
	"BOV",
	"BRL",
	"BSD",
	"BTN",
	"BWP",
	"BYN",
	"BZD",
	"CAD",
	"CDF",
	"CHE",
	"CHF",
	"CHW",
	"CLF",
	"CLP",
	"CNY",
	"COP",
	"COU",
	"CRC",
	"CUC",
	"CUP",
	"CVE",
	"CZK",
	"DJF",
	"DKK",
	"DOP",
	"DZD",
	"EGP",
	"ERN",
	"ETB",
	"EUR",
	"FJD",
	"FKP",
	"GBP",
	"GEL",
	"GHS",
	"GIP",
	"GMD",
	"GNF",
	"GTQ",
	"GYD",
	"HKD",
	"HNL",
	"HTG",
	"HUF",
	"IDR",
	"ILS",
	"INR",
	"IQD",
	"IRR",
	"ISK",
	"JMD",
	"JOD",
	"JPY",
	"KES",
	"KGS",
	"KHR",
	"KMF",
	"KPW",
	"KRW",
	"KWD",
	"KYD",
	"KZT",
	"LAK",
	"LBP",
	"LKR",
	"LRD",
	"LSL",
	"LYD",
	"MAD",
	"MDL",
	"MGA",
	"MKD",
	"MMK",
	"MNT",
	"MOP",
	"MRU",
	"MUR",
	"MVR",
	"MWK",
	"MXN",
	"MXV",
	"MYR",
	"MZN",
	"NAD",
	"NGN",
	"NIO",
	"NOK",
	"NPR",
	"NZD",
	"OMR",
	"PAB",
	"PEN",
	"PGK",
	"PHP",
	"PKR",
	"PLN",
	"PYG",
	"QAR",
	"RON",
	"RSD",
	"RUB",
	"RWF",
	"SAR",
	"SBD",
	"SCR",
	"SDG",
	"SEK",
	"SGD",
	"SHP",
	"SLE",
	"SLL",
	"SOS",
	"SRD",
	"SSP",
	"STN",
	"SVC",
	"SYP",
	"SZL",
	"THB",
	"TJS",
	"TMT",
	"TND",
	"TOP",
	"TRY",
	"TTD",
	"TWD",
	"TZS",
	"UAH",
	"UGX",
	"USD",
	"USN",
	"UYI",
	"UYU",
	"UYW",
	"UZS",
	"VED",
	"VES",
	"VND",
	"VUV",
	"WST",
	"XAF",
	"XAG",
	"XAU",
	"XBA",
	"XBB",
	"XBC",
	"XBD",
	"XCD",
	"XDR",
	"XOF",
	"XPD",
	"XPF",
	"XPT",
	"XSU",
	"XTS",
	"XUA",
	"XXX",
	"YER",
	"ZAR",
	"ZMW",
	"ZWL",
}
