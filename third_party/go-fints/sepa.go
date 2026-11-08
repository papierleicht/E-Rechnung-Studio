package fints

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const defaultPainDescriptor = "urn:iso:std:iso:20022:tech:xsd:pain.001.001.03"

var painDescriptorPattern = regexp.MustCompile(
	`^urn:iso:std:iso:20022:tech:xsd:pain\.001\.[0-9]{3}\.[0-9]{2}(?:\.xsd)?$`,
)

// SEPACreditTransfer submits one negotiated HKCCS SEPA credit transfer. The
// bank's advertised pain format is used and the operation is completed through
// the configured TAN handler.
func (c *Client) SEPACreditTransfer(
	ctx context.Context,
	transfer SEPACreditTransfer,
) (*TransferResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := validateTransfer(&transfer); err != nil {
		return nil, err
	}
	d, err := c.openDialog(ctx)
	if err != nil {
		return nil, err
	}
	defer c.endDialog(context.WithoutCancel(ctx), d)

	transfer.Debtor = normalizeAccount(transfer.Debtor)
	transfer.Debtor = resolveAccount(transfer.Debtor, d.accounts)
	if transfer.Debtor.IBAN != "" {
		if response, spaErr := c.executeCommand(ctx, d, "HKSPA:{n}:1'", "HKSPA"); spaErr == nil {
			transfer.Debtor = resolveAccount(
				transfer.Debtor,
				mergeAccounts(d.accounts, accountsFromHISPA(response)),
			)
		}
	}

	capability := c.capabilities["HKCCS"]
	if capability == nil || !containsInt(capability.Versions, 1) {
		return nil, errors.New("bank does not advertise HKCCS version 1 SEPA credit transfers")
	}
	if !supportsOperation(transfer.Debtor, "HKCCS") {
		return nil, errors.New("bank does not allow HKCCS for the selected debtor account")
	}
	descriptor, err := c.selectPainDescriptor(capability.Formats)
	if err != nil {
		return nil, err
	}
	pain, err := buildPain001(transfer, descriptor)
	if err != nil {
		return nil, err
	}
	command := fmt.Sprintf(
		"HKCCS:{n}:1+%s+%s+%s'",
		buildKTI(transfer.Debtor),
		escape(descriptor),
		binary(pain),
	)
	response, err := c.executeCommandWithTAN(ctx, d, command, "HKCCS", true)
	if err != nil {
		return nil, err
	}
	return &TransferResult{
		Accepted:       true,
		Operation:      "HKCCS",
		PainDescriptor: descriptor,
		Responses:      responseMessages(response),
	}, nil
}

func validateTransfer(transfer *SEPACreditTransfer) error {
	transfer.Debtor = normalizeAccount(transfer.Debtor)
	transfer.CreditorIBAN = normalizeIBAN(transfer.CreditorIBAN)
	transfer.CreditorBIC = strings.ToUpper(strings.TrimSpace(transfer.CreditorBIC))
	transfer.DebtorName = strings.TrimSpace(transfer.DebtorName)
	transfer.CreditorName = strings.TrimSpace(transfer.CreditorName)
	transfer.Purpose = strings.TrimSpace(transfer.Purpose)
	transfer.EndToEndReference = strings.TrimSpace(transfer.EndToEndReference)
	if !validIBAN(transfer.Debtor.IBAN) {
		return errors.New("debtor IBAN is invalid")
	}
	if !validIBAN(transfer.CreditorIBAN) {
		return errors.New("creditor IBAN is invalid")
	}
	if transfer.CreditorName == "" {
		return errors.New("creditor name is required")
	}
	if transfer.DebtorName == "" {
		transfer.DebtorName = transfer.Debtor.OwnerName
	}
	if transfer.DebtorName == "" {
		return errors.New("debtor name is required")
	}
	if transfer.Amount <= 0 {
		return errors.New("transfer amount must be positive")
	}
	if transfer.Currency == "" {
		transfer.Currency = "EUR"
	}
	transfer.Currency = strings.ToUpper(transfer.Currency)
	if transfer.Currency != "EUR" {
		return errors.New("HKCCS SEPA credit transfers require EUR")
	}
	if len([]rune(transfer.Purpose)) > 140 {
		return errors.New("transfer purpose must not exceed 140 characters")
	}
	if transfer.EndToEndReference == "" {
		transfer.EndToEndReference = "NOTPROVIDED"
	}
	if len(transfer.EndToEndReference) > 35 {
		return errors.New("end-to-end reference must not exceed 35 characters")
	}
	if transfer.ExecutionDate.IsZero() {
		transfer.ExecutionDate = time.Now()
	}
	return nil
}

func (c *Client) selectPainDescriptor(operationFormats []string) (string, error) {
	var candidates []string
	for _, source := range [][]string{
		operationFormats,
		capabilityFormats(c.capabilities["HKSPA"]),
	} {
		for _, descriptor := range source {
			descriptor = strings.TrimSuffix(descriptor, ".xsd")
			if painDescriptorPattern.MatchString(descriptor) {
				candidates = appendUniqueString(candidates, descriptor)
			}
		}
	}
	if len(candidates) == 0 {
		// Old BPDs sometimes advertise HKCCS without repeating the general
		// HISPAS format list. The broadly supported pain.001.001.03 is the
		// interoperable fallback.
		return defaultPainDescriptor, nil
	}
	// Prefer the newest advertised format. buildPain001 adapts the structural
	// differences relevant to the supported pain.001 generations.
	return candidates[len(candidates)-1], nil
}

func capabilityFormats(capability *Capability) []string {
	if capability == nil {
		return nil
	}
	return capability.Formats
}

func buildPain001(transfer SEPACreditTransfer, descriptor string) ([]byte, error) {
	if !painDescriptorPattern.MatchString(descriptor) {
		return nil, fmt.Errorf("unsupported pain.001 descriptor %q", descriptor)
	}
	now := time.Now().UTC()
	reference, err := randomReference()
	if err != nil {
		return nil, err
	}
	messageID := "FINTS-" + now.Format("20060102T150405") + "-" + reference
	paymentID := "PMT-" + reference
	newGeneration := painGeneration(descriptor) >= 9
	bicElement := "BIC"
	if newGeneration {
		bicElement = "BICFI"
	}

	var output bytes.Buffer
	output.WriteString(xml.Header)
	output.WriteString(`<Document xmlns="`)
	xmlEscape(&output, descriptor)
	output.WriteString(`"><CstmrCdtTrfInitn><GrpHdr><MsgId>`)
	xmlEscape(&output, messageID)
	output.WriteString(`</MsgId><CreDtTm>`)
	output.WriteString(now.Format("2006-01-02T15:04:05Z"))
	output.WriteString(`</CreDtTm><NbOfTxs>1</NbOfTxs><CtrlSum>`)
	output.WriteString(formatMinorUnits(transfer.Amount))
	output.WriteString(`</CtrlSum><InitgPty><Nm>`)
	xmlEscape(&output, transfer.DebtorName)
	output.WriteString(`</Nm></InitgPty></GrpHdr><PmtInf><PmtInfId>`)
	xmlEscape(&output, paymentID)
	output.WriteString(`</PmtInfId><PmtMtd>TRF</PmtMtd><BtchBookg>false</BtchBookg>`)
	output.WriteString(`<NbOfTxs>1</NbOfTxs><CtrlSum>`)
	output.WriteString(formatMinorUnits(transfer.Amount))
	output.WriteString(`</CtrlSum><PmtTpInf><SvcLvl><Cd>SEPA</Cd></SvcLvl></PmtTpInf><ReqdExctnDt>`)
	if newGeneration {
		output.WriteString(`<Dt>`)
	}
	output.WriteString(transfer.ExecutionDate.Format(time.DateOnly))
	if newGeneration {
		output.WriteString(`</Dt>`)
	}
	output.WriteString(`</ReqdExctnDt><Dbtr><Nm>`)
	xmlEscape(&output, transfer.DebtorName)
	output.WriteString(`</Nm></Dbtr><DbtrAcct><Id><IBAN>`)
	xmlEscape(&output, transfer.Debtor.IBAN)
	output.WriteString(`</IBAN></Id></DbtrAcct>`)
	writeAgent(&output, "DbtrAgt", bicElement, transfer.Debtor.BIC)
	output.WriteString(`<ChrgBr>SLEV</ChrgBr><CdtTrfTxInf><PmtId><EndToEndId>`)
	xmlEscape(&output, transfer.EndToEndReference)
	output.WriteString(`</EndToEndId></PmtId><Amt><InstdAmt Ccy="`)
	xmlEscape(&output, transfer.Currency)
	output.WriteString(`">`)
	output.WriteString(formatMinorUnits(transfer.Amount))
	output.WriteString(`</InstdAmt></Amt>`)
	writeAgent(&output, "CdtrAgt", bicElement, transfer.CreditorBIC)
	output.WriteString(`<Cdtr><Nm>`)
	xmlEscape(&output, transfer.CreditorName)
	output.WriteString(`</Nm></Cdtr><CdtrAcct><Id><IBAN>`)
	xmlEscape(&output, transfer.CreditorIBAN)
	output.WriteString(`</IBAN></Id></CdtrAcct>`)
	if transfer.Purpose != "" {
		output.WriteString(`<RmtInf><Ustrd>`)
		xmlEscape(&output, transfer.Purpose)
		output.WriteString(`</Ustrd></RmtInf>`)
	}
	output.WriteString(`</CdtTrfTxInf></PmtInf></CstmrCdtTrfInitn></Document>`)
	return output.Bytes(), nil
}

func painGeneration(descriptor string) int {
	parts := strings.Split(descriptor, ".")
	if len(parts) == 0 {
		return 0
	}
	var generation int
	_, _ = fmt.Sscanf(parts[len(parts)-1], "%d", &generation)
	return generation
}

func writeAgent(output *bytes.Buffer, element, bicElement, bic string) {
	if bic == "" {
		return
	}
	output.WriteString("<" + element + "><FinInstnId><" + bicElement + ">")
	xmlEscape(output, bic)
	output.WriteString("</" + bicElement + "></FinInstnId></" + element + ">")
}

func xmlEscape(output *bytes.Buffer, value string) {
	_ = xml.EscapeText(output, []byte(value))
}

func responseMessages(response []segment) []ResponseMessage {
	var result []ResponseMessage
	for _, kind := range []string{"HIRMG", "HIRMS"} {
		for _, s := range findSegments(response, kind) {
			for index := range s.Fields {
				values := s.Fields[index].components
				if len(values) == 0 || len(values[0]) != 4 {
					continue
				}
				message := ResponseMessage{
					Code:             values[0],
					Message:          valuesAt(values, 2),
					Segment:          s.Header.Reference,
					ReferenceElement: valuesAt(values, 1),
				}
				if len(values) > 3 {
					message.Params = append([]string(nil), values[3:]...)
				}
				result = append(result, message)
			}
		}
	}
	return result
}

func containsInt(values []int, expected int) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
