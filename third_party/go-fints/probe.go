package fints

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ProbeConfig configures an anonymous FinTS 3.0 bank-parameter inquiry. There
// are intentionally no credentials or account identifiers in this API.
type ProbeConfig struct {
	Endpoint       string
	BankCode       string
	ProductID      string
	ProductVersion string
	// Timeout bounds the entire inquiry, including best-effort dialog cleanup.
	// Zero selects 30 seconds, including when HTTPClient has no timeout.
	Timeout    time.Duration
	HTTPClient *http.Client
	// AllowHTTP is for local test servers. Production endpoints must use HTTPS.
	AllowHTTP bool
}

// ProbeResult contains only public bank parameters. AdvertisedProtocols are
// declarations in HIBPA; they do not demonstrate that those protocols work.
// ResponseProtocol is the actual protocol of the validated response. Successful
// probing does not establish authenticated account or transaction access.
type ProbeResult struct {
	ResponseProtocol    string       `json:"response_protocol"`
	AdvertisedProtocols []string     `json:"advertised_protocols"`
	BankName            string       `json:"bank_name"`
	BPDVersion          int          `json:"bpd_version"`
	TANMethods          []TANMethod  `json:"tan_methods"`
	Capabilities        []Capability `json:"capabilities"`
	ResponseCodes       []string     `json:"response_codes"`
}

// Probe sends an unsigned, unencrypted anonymous HKIDN/HKVVB inquiry. It does
// not log in, synchronize a customer system ID, request accounts, or negotiate
// alternate protocols. A successful result requires a valid FinTS 3.0 response,
// a positive global response, and bank parameters matching the requested BLZ.
func Probe(ctx context.Context, config ProbeConfig) (ProbeResult, error) {
	client, err := newProbeClient(config)
	if err != nil {
		return ProbeResult{}, err
	}
	if ctx == nil {
		return ProbeResult{}, errors.New("FinTS probe requires a context")
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	request, err := buildProbeMessage("0", 1, []string{
		fmt.Sprintf("HKIDN:2:2+280:%s+9999999999+0+0'", config.BankCode),
		fmt.Sprintf("HKVVB:3:3+0+0+1+%s+%s'", escape(client.config.ProductID), escape(client.config.ProductVersion)),
	})
	if err != nil {
		return ProbeResult{}, err
	}
	data, err := client.post(ctx, request)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("anonymous FinTS inquiry: %w", err)
	}
	response, err := parseProbeEnvelope(data, 1)
	if err != nil {
		return ProbeResult{}, err
	}
	// Cleanup never uses the authenticated Client.endDialog implementation.
	// It is attempted even if a valid envelope carries an error or wrong BPD.
	defer closeProbeDialog(ctx, client, component(response[0], 2, 0))

	codes, err := probeResponseCodes(response)
	if err != nil {
		return ProbeResult{}, err
	}
	bpd := findSegments(response, "HIBPA")
	if len(bpd) != 1 || bpd[0].Header.Version != 3 || len(bpd[0].Fields) < 6 {
		return ProbeResult{}, errors.New("anonymous FinTS inquiry did not return a valid HIBPA3 bank-parameter block")
	}
	parameters := bpd[0]
	if component(parameters, 1, 0) != "280" || component(parameters, 1, 1) != config.BankCode {
		return ProbeResult{}, errors.New("anonymous FinTS bank parameters do not match the requested bank code")
	}
	versionText := component(parameters, 0, 0)
	if !probeDigits(versionText, 1, 3) {
		return ProbeResult{}, errors.New("anonymous FinTS bank parameters contain an invalid BPD version")
	}
	bpdVersion, _ := strconv.Atoi(versionText)
	bankName := component(parameters, 2, 0)
	if strings.TrimSpace(bankName) == "" {
		return ProbeResult{}, errors.New("anonymous FinTS bank parameters omit the bank name")
	}
	protocols := make([]string, 0, len(parameters.Fields[5].components))
	for _, protocol := range parameters.Fields[5].components {
		if !probeDigits(protocol, 3, 3) {
			return ProbeResult{}, errors.New("anonymous FinTS bank parameters contain invalid advertised protocols")
		}
		protocols = appendUniqueString(protocols, protocol)
	}
	if len(protocols) == 0 {
		return ProbeResult{}, errors.New("anonymous FinTS bank parameters omit advertised protocols")
	}

	client.updateParameters(response)
	methods := make([]TANMethod, 0, len(client.tanMethods))
	for _, method := range client.tanMethods {
		methods = append(methods, method)
	}
	sort.Slice(methods, func(i, j int) bool { return methods[i].SecurityFunction < methods[j].SecurityFunction })
	capabilities := make([]Capability, 0, len(client.capabilities))
	for _, capability := range client.capabilities {
		capabilities = append(capabilities, *capability)
	}
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i].Operation < capabilities[j].Operation })
	return ProbeResult{
		ResponseProtocol:    component(response[0], 1, 0),
		AdvertisedProtocols: protocols,
		BankName:            bankName,
		BPDVersion:          bpdVersion,
		TANMethods:          methods,
		Capabilities:        capabilities,
		ResponseCodes:       codes,
	}, nil
}

func newProbeClient(config ProbeConfig) (*Client, error) {
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil {
		// URL parse errors can contain the original input; do not echo credentials.
		return nil, errors.New("invalid FinTS probe endpoint")
	}
	if endpoint.Scheme != "https" && !(config.AllowHTTP && endpoint.Scheme == "http") {
		return nil, errors.New("FinTS probe endpoint must use HTTPS")
	}
	if endpoint.Hostname() == "" || endpoint.Opaque != "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" || strings.Contains(config.Endpoint, "#") {
		return nil, errors.New("FinTS probe endpoint requires a host and must not contain credentials, a query, or a fragment")
	}
	if !probeDigits(config.BankCode, 8, 8) {
		return nil, errors.New("FinTS probe bank code must contain exactly eight digits")
	}
	if config.ProductVersion == "" {
		config.ProductVersion = defaultProduct
	}
	if err := validateProductMetadata(config.ProductID, config.ProductVersion); err != nil {
		return nil, err
	}
	if config.Timeout < 0 {
		return nil, errors.New("FinTS probe timeout must not be negative")
	}
	httpClient := &http.Client{}
	if config.HTTPClient != nil {
		copy := *config.HTTPClient
		httpClient = &copy
	}
	// Do not attach ambient cookies from a caller-supplied client.
	httpClient.Jar = nil
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return errors.New("FinTS probe redirects are not allowed")
	}
	return &Client{
		config: Config{
			Endpoint:       config.Endpoint,
			BankCode:       config.BankCode,
			CountryCode:    "280",
			ProductID:      config.ProductID,
			ProductVersion: config.ProductVersion,
		},
		httpClient:      httpClient,
		tanMethods:      make(map[string]TANMethod),
		tanPolling:      make(map[string]decoupledPollingParameters),
		tanRequirements: make(map[string]bool),
		capabilities:    make(map[string]*Capability),
	}, nil
}

func buildProbeMessage(dialogID string, messageNumber int, commands []string) ([]byte, error) {
	message := fmt.Sprintf("HNHBK:1:3+000000000000+300+%s+%d'", escape(dialogID), messageNumber) +
		strings.Join(commands, "") + fmt.Sprintf("HNHBS:%d:1+%d'", len(commands)+2, messageNumber)
	encoded, err := encodeWireText([]byte(message))
	if err != nil {
		return nil, fmt.Errorf("build anonymous FinTS inquiry: %w", err)
	}
	return bytes.Replace(encoded, []byte("000000000000"), []byte(fmt.Sprintf("%012d", len(encoded))), 1), nil
}

func parseProbeEnvelope(data []byte, messageNumber int) ([]segment, error) {
	if !bytes.HasPrefix(data, []byte("HNHBK:1:3+")) || !bytes.HasSuffix(data, []byte("'")) {
		return nil, errors.New("anonymous FinTS response is not a complete FinTS 3.0 envelope")
	}
	segments, err := parseSegments(data)
	if err != nil {
		return nil, fmt.Errorf("parse anonymous FinTS response: %w", err)
	}
	if len(segments) < 3 {
		return nil, errors.New("anonymous FinTS response is incomplete")
	}
	header, trailer := segments[0], segments[len(segments)-1]
	sizeText := component(header, 0, 0)
	size, _ := strconv.Atoi(sizeText)
	if !probeDigits(sizeText, 12, 12) || size != len(data) || component(header, 1, 0) != "300" {
		return nil, errors.New("anonymous FinTS response has an invalid size or protocol version")
	}
	dialogID := component(header, 2, 0)
	if dialogID == "" || len(dialogID) > 30 || component(header, 3, 0) != strconv.Itoa(messageNumber) {
		return nil, errors.New("anonymous FinTS response has an invalid dialog or message number")
	}
	if len(header.Fields) > 4 && (component(header, 4, 0) != dialogID || component(header, 4, 1) != strconv.Itoa(messageNumber)) {
		return nil, errors.New("anonymous FinTS response has a mismatched message reference")
	}
	if trailer.Header.Type != "HNHBS" || trailer.Header.Version != 1 || component(trailer, 0, 0) != strconv.Itoa(messageNumber) {
		return nil, errors.New("anonymous FinTS response has a mismatched message trailer")
	}
	for index, s := range segments {
		if s.Header.Number != index+1 || (s.Header.Type == "HNHBK" && index != 0) || (s.Header.Type == "HNHBS" && index != len(segments)-1) {
			return nil, errors.New("anonymous FinTS response has invalid segment ordering")
		}
		switch s.Header.Type {
		case "HNVSK", "HNVSD", "HNSHK", "HNSHA":
			return nil, errors.New("anonymous FinTS response unexpectedly contains a security envelope")
		}
	}
	return segments, nil
}

func probeResponseCodes(response []segment) ([]string, error) {
	codes := make([]string, 0)
	positiveGlobal := false
	var errorCodes []string
	for _, s := range response {
		if s.Header.Type != "HIRMG" && s.Header.Type != "HIRMS" {
			continue
		}
		for _, field := range s.Fields {
			if len(field.components) == 0 || !probeDigits(field.components[0], 4, 4) {
				return nil, errors.New("anonymous FinTS response contains a malformed return code")
			}
			code := field.components[0]
			codes = appendUniqueString(codes, code)
			if code[0] == '9' {
				errorCodes = appendUniqueString(errorCodes, code)
			}
			if s.Header.Type == "HIRMG" && code[0] == '0' {
				positiveGlobal = true
			}
		}
	}
	if len(errorCodes) != 0 {
		return nil, fmt.Errorf("anonymous FinTS inquiry rejected (codes %s)", strings.Join(errorCodes, ", "))
	}
	if !positiveGlobal {
		return nil, errors.New("anonymous FinTS response has no positive HIRMG acknowledgement")
	}
	return codes, nil
}

func closeProbeDialog(ctx context.Context, client *Client, dialogID string) {
	if dialogID == "0" || ctx.Err() != nil {
		return
	}
	closeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	request, err := buildProbeMessage(dialogID, 2, []string{fmt.Sprintf("HKEND:2:1+%s'", escape(dialogID))})
	if err != nil {
		return
	}
	_, _ = client.post(closeCtx, request)
}

func probeDigits(value string, minimum, maximum int) bool {
	if len(value) < minimum || len(value) > maximum {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
