package fints

import (
	"bytes"
	"context"
	"encoding/base64"
	binaryencoding "encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type decoupledPollingParameters struct {
	maxPolls                 int
	firstDelay               time.Duration
	nextDelay                time.Duration
	automaticPollingDisabled bool
}

// TANMethods returns the PIN/TAN mechanisms advertised by the bank. The
// client's configured mechanism is selected automatically when it is empty.
func (c *Client) TANMethods(ctx context.Context) ([]TANMethod, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureAuthentication(ctx); err != nil {
		return nil, err
	}
	methods := make([]TANMethod, 0, len(c.tanMethods))
	for _, method := range c.tanMethods {
		methods = append(methods, method)
	}
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].SecurityFunction < methods[j].SecurityFunction
	})
	return methods, nil
}

// TANMedia returns the active TAN devices or apps advertised by the bank.
func (c *Client) TANMedia(ctx context.Context) ([]TANMedium, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.tanMediaLocked(ctx)
}

// SetTANMedium selects the exact, case-sensitive medium name used in HKTAN.
func (c *Client) SetTANMedium(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config.TANMedium = strings.TrimSpace(name)
}

func (c *Client) ensureAuthentication(ctx context.Context) error {
	if c.systemID == "0" {
		if err := c.synchronize(ctx); err != nil {
			return err
		}
	} else if !c.parametersReady {
		if err := c.bootstrapParameters(ctx); err != nil {
			return err
		}
	}

	if c.config.SecurityFunction == "" {
		c.config.SecurityFunction = c.selectTANMethod()
	}
	if c.config.SecurityFunction == "" {
		c.config.SecurityFunction = defaultSecurity
	}
	if len(c.allowedSecurity) > 0 && !containsString(c.allowedSecurity, c.config.SecurityFunction) {
		return failureWithResponse(errors.New("selected security function is not allowed for this user; select a TAN method advertised by the bank"), "select_tan_method", "tan_method_not_allowed", nil, nil)
	}
	return nil
}

// bootstrapParameters obtains BPD/HITANS when a caller supplied a cached
// system ID and synchronization can therefore be skipped.
func (c *Client) bootstrapParameters(ctx context.Context) (err error) {
	defer func() { err = failureWithResponse(err, "load_parameters", "", nil, nil) }()
	d := &dialog{id: "0", nextMessage: 1, systemID: c.systemID, security: defaultSecurity}
	commands := []string{
		fmt.Sprintf("HKIDN:{n}:2+%s:%s+%s+%s+1'",
			escape(c.config.CountryCode), escape(c.config.BankCode), escape(c.config.CustomerID), escape(c.systemID)),
		fmt.Sprintf("HKVVB:{n}:3+0+0+0+%s+%s'",
			escape(c.config.ProductID), escape(c.config.ProductVersion)),
	}
	response, err := c.sendCommands(ctx, d, commands)
	if err != nil {
		return fmt.Errorf("load FinTS parameters: %w", err)
	}
	d.id = responseDialogID(response)
	c.updateParameters(response)
	c.endDialog(ctx, d)
	return nil
}

func (c *Client) selectTANMethod() string {
	for _, preferred := range c.config.Compatibility.PreferredSecurityFunctions {
		if len(c.allowedSecurity) > 0 && !containsString(c.allowedSecurity, preferred) {
			continue
		}
		if _, ok := c.tanMethods[preferred]; ok {
			if c.config.HKTANVersion == 0 {
				c.config.HKTANVersion = c.tanMethods[preferred].HKTANVersion
			}
			return preferred
		}
	}
	var best *TANMethod
	for _, method := range c.tanMethods {
		if len(c.allowedSecurity) > 0 && !containsString(c.allowedSecurity, method.SecurityFunction) {
			continue
		}
		candidate := method
		if best == nil {
			best = &candidate
			continue
		}
		candidateNumber, _ := strconv.Atoi(candidate.SecurityFunction)
		bestNumber, _ := strconv.Atoi(best.SecurityFunction)
		if candidateNumber > bestNumber {
			best = &candidate
		}
	}
	if best == nil {
		return defaultSecurity
	}
	if c.config.HKTANVersion == 0 {
		c.config.HKTANVersion = best.HKTANVersion
	}
	return best.SecurityFunction
}

func (c *Client) hktanVersion() int {
	if c.config.HKTANVersion >= 5 && c.config.HKTANVersion <= 7 {
		return c.config.HKTANVersion
	}
	if method, ok := c.tanMethods[c.config.SecurityFunction]; ok &&
		method.HKTANVersion >= 5 && method.HKTANVersion <= 7 {
		return method.HKTANVersion
	}
	return 6
}

func (c *Client) updateTANParameters(response []segment) {
	if c.tanPolling == nil {
		c.tanPolling = make(map[string]decoupledPollingParameters)
	}
	allowed := allowedSecurityFunctions(response)
	if len(allowed) > 0 {
		c.allowedSecurity = append([]string(nil), allowed...)
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, security := range allowed {
		allowedSet[security] = true
	}

	for _, s := range findSegments(response, "HITANS") {
		values := flattenedComponents(s)
		methodWidth := 1
		switch s.Header.Version {
		case 5:
			methodWidth = 22
		case 6:
			methodWidth = 21
		case 7:
			methodWidth = 26
		}
		for index := 0; index < len(values); index++ {
			value := values[index]
			security, err := strconv.Atoi(value)
			if len(value) != 3 || err != nil || security < 900 || security > 997 || valuesAt(values, index+1) != "2" {
				continue
			}
			start := index
			// Do not rescan fields inside a method. In HITANS7 the maximum
			// polling count can look like a method code followed by process 2
			// when the bank's first polling delay happens to be two seconds.
			index += methodWidth - 1
			if len(allowedSet) > 0 && !allowedSet[value] {
				continue
			}
			name := valuesAt(values, start+5)
			if name == "" {
				name = valuesAt(values, start+3)
			}
			if name == "" {
				name = "PIN/TAN " + value
			}
			mediumRequired := valuesAt(values, start+18) == "2"
			if containsString(
				c.config.Compatibility.TANMediumRequiredForFunctions,
				value,
			) {
				mediumRequired = true
			}
			c.tanMethods[value] = TANMethod{
				SecurityFunction: value,
				Name:             name,
				HKTANVersion:     s.Header.Version,
				TANProcess:       "2",
				MediumRequired:   mediumRequired,
				Decoupled:        s.Header.Version >= 7 && strings.EqualFold(valuesAt(values, start+3), "Decoupled"),
			}
			if s.Header.Version >= 7 {
				c.tanPolling[value] = decoupledPollingParameters{
					maxPolls:                 positiveInt(valuesAt(values, start+21)),
					firstDelay:               secondsDuration(valuesAt(values, start+22)),
					nextDelay:                secondsDuration(valuesAt(values, start+23)),
					automaticPollingDisabled: valuesAt(values, start+25) == "N",
				}
			}
		}
	}
	// Some parameter responses list an allowed function in response 3920
	// before delivering a complete HITANS block. Retain the usable mechanism
	// and refine it on the next BPD response.
	for _, security := range allowed {
		if security == defaultSecurity {
			continue
		}
		if _, exists := c.tanMethods[security]; exists {
			continue
		}
		name := "PIN/TAN " + security
		mediumRequired := containsString(
			c.config.Compatibility.TANMediumRequiredForFunctions,
			security,
		)
		version := c.config.HKTANVersion
		if version < 5 || version > 7 {
			version = 6
		}
		c.tanMethods[security] = TANMethod{
			SecurityFunction: security,
			Name:             name,
			HKTANVersion:     version,
			TANProcess:       "2",
			MediumRequired:   mediumRequired,
		}
	}

	for _, s := range findSegments(response, "HIPINS") {
		values := flattenedComponents(s)
		for index, value := range values {
			// Standard order codes such as HKCCS have five characters;
			// the FinTS segment-code field also permits six-character codes.
			if (len(value) == 5 || len(value) == 6) && strings.HasPrefix(value, "HK") {
				required := valuesAt(values, index+1)
				if required == "J" || required == "N" {
					c.tanRequirements[value] = required == "J"
				}
			}
		}
	}
	for _, s := range findSegments(response, "HITABS") {
		if s.Header.Version == 4 || s.Header.Version == 5 {
			c.hktabVersion = s.Header.Version
		}
	}
}

func positiveInt(value string) int {
	number, err := strconv.Atoi(value)
	if err != nil || number <= 0 {
		return 0
	}
	return number
}

func secondsDuration(value string) time.Duration {
	number := positiveInt(value)
	if number == 0 {
		return 0
	}
	return time.Duration(number) * time.Second
}

func allowedSecurityFunctions(response []segment) []string {
	seen := make(map[string]bool)
	var result []string
	for _, kind := range []string{"HIRMG", "HIRMS"} {
		for _, s := range findSegments(response, kind) {
			for index := range s.Fields {
				if component(s, index, 0) != "3920" {
					continue
				}
				for componentIndex := 3; componentIndex < len(s.Fields[index].components); componentIndex++ {
					value := component(s, index, componentIndex)
					if len(value) == 3 && !seen[value] {
						seen[value] = true
						result = append(result, value)
					}
				}
			}
		}
	}
	return result
}

func flattenedComponents(s segment) []string {
	var result []string
	for _, f := range s.Fields {
		result = append(result, f.components...)
	}
	return result
}

func valuesAt(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}

func buildHKTAN(version int, process, segmentType, taskReference, medium string) string {
	if version < 5 || version > 7 {
		version = 6
	}
	fields := []string{escape(process)}
	switch process {
	case "4":
		fields = append(fields, escape(segmentType), "", "", "", "", "", "", "", "", escape(medium))
	case "2":
		fields = append(fields, "", "", "", escape(taskReference), "N")
	case "S":
		fields = append(fields, "", "", "", escape(taskReference), "N")
	}
	for len(fields) > 1 && fields[len(fields)-1] == "" {
		fields = fields[:len(fields)-1]
	}
	return fmt.Sprintf("HKTAN:{n}:%d+%s'", version, strings.Join(fields, "+"))
}

// Feedback may refer to HKTAN itself or to the order protected by process 4.
// Derive both references from this request, never from an unrelated response.
func submittedTANReferences(commands []string) (tanReference, orderReference int) {
	for index, command := range commands {
		if !strings.HasPrefix(command, "HKTAN:") {
			continue
		}
		if tanReference != 0 {
			return 0, 0
		} // No unambiguous single TAN order.
		tanReference = 3 + index
		_, fields, _ := strings.Cut(command, "+")
		parts := strings.Split(strings.TrimSuffix(fields, "'"), "+")
		if len(parts) < 2 || parts[0] != "4" || !diagnosticSegmentType(parts[1]) {
			continue
		}
		for preceding := 0; preceding < index; preceding++ {
			if !strings.HasPrefix(commands[preceding], parts[1]+":") {
				continue
			}
			if orderReference != 0 {
				orderReference = 0
				break
			}
			orderReference = 3 + preceding
		}
	}
	return
}

func (c *Client) executeCommand(ctx context.Context, d *dialog, command, kind string) ([]segment, error) {
	return c.executeCommandWithTAN(ctx, d, command, kind, false)
}

func (c *Client) executeCommandWithTAN(
	ctx context.Context,
	d *dialog,
	command, kind string,
	forceTAN bool,
) ([]segment, error) {
	commands := []string{command}
	tanAttached := d.security != defaultSecurity &&
		(forceTAN ||
			c.tanRequirements[kind] ||
			containsString(c.config.Compatibility.ForceTANForOperations, kind))
	if tanAttached {
		commands = append(commands, buildHKTAN(c.hktanVersion(), "4", kind, "", c.config.TANMedium))
	}
	response, err := c.sendCommands(ctx, d, commands)
	retriedWithHKTAN := false
	if err != nil && !tanAttached && d.security != defaultSecurity && strongAuthenticationRequired(err) {
		// Some banks demand SCA dynamically even when HIPINS/UPD marks the
		// operation as not requiring a TAN. The authentication error means the
		// order was not executed, so retry it once with process 4 attached.
		commands = append(commands, buildHKTAN(c.hktanVersion(), "4", kind, "", c.config.TANMedium))
		response, err = c.sendCommands(ctx, d, commands)
		tanAttached = true
		retriedWithHKTAN = true
	}
	if err != nil && tanAttached && bankErrorHasCode(err, "9010") {
		media, mediaErr := c.tanMediaLocked(ctx)
		if mediaErr != nil {
			return nil, fmt.Errorf("refresh TAN media after bank rejected %q: %w", c.config.TANMedium, mediaErr)
		}
		medium, selectErr := selectRefreshedTANMedium(c.config.TANMedium, media)
		if selectErr != nil {
			return nil, selectErr
		}
		c.config.TANMedium = medium
		commands = []string{
			command,
			buildHKTAN(c.hktanVersion(), "4", kind, "", medium),
		}
		response, err = c.sendCommands(ctx, d, commands)
	}
	if err != nil {
		if retriedWithHKTAN {
			return nil, fmt.Errorf("%s retry with HKTAN after bank required strong authentication: %w", kind, err)
		}
		return nil, err
	}
	followup, err := c.completeTAN(ctx, d, response)
	if err != nil {
		return nil, fmt.Errorf("complete %s strong authentication: %w", kind, err)
	}
	return append(response, followup...), nil
}

func strongAuthenticationRequired(err error) bool {
	// 9370 is used by several Sparkasse endpoints when a validly signed order
	// still lacks the order-specific strong authentication represented by
	// HKTAN. Like 9075, the rejected order was not executed and can safely be
	// retried with process 4 attached.
	return bankErrorHasCode(err, "9075") || bankErrorHasCode(err, "9370")
}

func bankErrorHasCode(err error, code string) bool {
	var bankErr *BankError
	if !errors.As(err, &bankErr) {
		return false
	}
	if bankErr.Code == code {
		return true
	}
	for _, detail := range bankErr.Details {
		if detail.Code == code {
			return true
		}
	}
	return false
}

func selectRefreshedTANMedium(configured string, media []TANMedium) (string, error) {
	for _, candidate := range media {
		if candidate.Name == configured {
			return candidate.Name, nil
		}
	}
	for _, candidate := range media {
		if strings.EqualFold(candidate.Name, configured) {
			return candidate.Name, nil
		}
	}
	if len(media) == 1 {
		return media[0].Name, nil
	}
	return "", &TANMediumRequiredError{Media: media}
}

func (c *Client) completeTAN(ctx context.Context, d *dialog, response []segment) (result []segment, err error) {
	defer func() { err = failureWithResponse(err, "complete_tan", "", nil, response) }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	challenge, code, required := tanChallenge(response, c.config.SecurityFunction, c.config.TANMedium, d.lastTANRequest, d.lastTANOrder)
	if !required {
		return nil, nil
	}
	// The response code decides this challenge's flow. A decoupled-capable
	// method can still return 0030 and require a manually entered TAN.
	if challenge.TaskReference == "" {
		if code == "" {
			return nil, failureWithResponse(errors.New("bank TAN response could not be assigned unambiguously to the request"), "complete_tan", "ambiguous_tan_response", nil, response)
		}
		return nil, failureWithResponse(errors.New("bank requested a TAN without returning a task reference"), "complete_tan", "missing_tan_reference", nil, response)
	}

	if challenge.Decoupled {
		if c.config.TANHandler == nil && !c.config.AutoPoll {
			return nil, &TANRequiredError{Code: code, Challenge: challenge.Challenge, Details: &challenge}
		}
		if c.config.TANHandler != nil {
			if _, err := c.config.TANHandler(ctx, challenge); err != nil {
				return nil, fmt.Errorf("handle TAN challenge: %w", err)
			}
		}
		result, err := c.pollDecoupled(ctx, d, challenge)
		return result, tanErrorWithResponse(err, response)
	}

	if c.config.TANHandler == nil {
		return nil, &TANRequiredError{Code: code, Challenge: challenge.Challenge, Details: &challenge}
	}
	tan, err := c.config.TANHandler(ctx, challenge)
	if err != nil {
		return nil, fmt.Errorf("handle TAN challenge: %w", err)
	}
	if tan == "" {
		return nil, failureWithResponse(errors.New("TAN handler returned an empty TAN"), "complete_tan", "empty_tan", nil, nil)
	}
	version := challenge.HKTANVersion
	if version < 5 || version > 7 {
		version = c.hktanVersion()
	}
	result, err = c.sendCommandsWithTAN(ctx, d, []string{
		buildHKTAN(version, "2", "", challenge.TaskReference, ""),
	}, tan)
	return result, tanErrorWithResponse(err, response)
}

func (c *Client) pollDecoupled(ctx context.Context, d *dialog, challenge TANChallenge) ([]segment, error) {
	if c.tanPolling[c.config.SecurityFunction].automaticPollingDisabled {
		return nil, failureWithResponse(errors.New("bank does not permit automatic polling for the selected TAN method"), "poll_tan", "automatic_polling_disabled", nil, nil)
	}
	version := challenge.HKTANVersion
	if version < 5 || version > 7 {
		version = c.hktanVersion()
	}
	// HKTAN version 7 introduced the decoupled status process S. Older segment
	// versions use process 2; applications can still override either behavior
	// for endpoints that deviate from their advertised parameters.
	process := "2"
	if version >= 7 {
		process = "S"
	}
	switch strings.ToUpper(strings.TrimSpace(
		c.config.Compatibility.DecoupledPollProcess,
	)) {
	case "2":
		process = "2"
	case "S":
		process = "S"
	}
	firstDelay, nextDelay, maxPolls := c.decoupledPollingSchedule()
	for attempt := 1; attempt <= maxPolls; attempt++ {
		delay := nextDelay
		if attempt == 1 {
			delay = firstDelay
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if c.config.OnTANPoll != nil {
			progress := challenge
			progress.PollAttempt = attempt
			// Isolate payload slices from application callbacks.
			progress.ChallengeHHD = append([]byte(nil), challenge.ChallengeHHD...)
			progress.Image = append([]byte(nil), challenge.Image...)
			if err := c.config.OnTANPoll(ctx, progress); err != nil {
				return nil, fmt.Errorf("decoupled approval canceled: %w", err)
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}

		messageNumber := d.nextMessage
		response, err := c.sendCommands(ctx, d, []string{
			buildHKTAN(version, process, "", challenge.TaskReference, ""),
		})
		if err != nil {
			return nil, fmt.Errorf(
				"poll decoupled approval attempt %d with dialog message %d: %w",
				attempt,
				messageNumber,
				err,
			)
		}
		if hasResponseCode(response, "3956") {
			continue
		}
		if _, _, stillRequired := tanChallenge(
			response, c.config.SecurityFunction, c.config.TANMedium, d.lastTANRequest, d.lastTANOrder,
		); stillRequired {
			continue
		}
		return response, nil
	}
	return nil, failureWithResponse(fmt.Errorf("decoupled approval was not completed after %d polls", maxPolls), "poll_tan", "poll_limit_reached", nil, nil)
}

func (c *Client) decoupledPollingSchedule() (time.Duration, time.Duration, int) {
	parameters := c.tanPolling[c.config.SecurityFunction]
	firstDelay := maxDuration(c.config.PollInterval, parameters.firstDelay)
	nextDelay := maxDuration(c.config.PollInterval, parameters.nextDelay)
	maxPolls := c.config.MaxPolls
	if parameters.maxPolls > 0 && parameters.maxPolls < maxPolls {
		maxPolls = parameters.maxPolls
	}
	return firstDelay, nextDelay, maxPolls
}

func maxDuration(left, right time.Duration) time.Duration {
	if right > left {
		return right
	}
	return left
}

func tanChallenge(response []segment, method, medium string, expectedReferences ...int) (TANChallenge, string, bool) {
	expected := 0
	protectedOrder := 0
	if len(expectedReferences) > 0 {
		expected = expectedReferences[0]
	}
	if expected > 0 && len(expectedReferences) > 1 {
		protectedOrder = expectedReferences[1]
	}
	candidates := findSegments(response, "HITAN")
	if expected > 0 {
		var exact []segment
		for _, candidate := range candidates {
			if candidate.Header.Reference == expected {
				exact = append(exact, candidate)
			}
		}
		if len(exact) > 0 {
			candidates = exact
		}
		if len(candidates) > 1 && hasResponseCode(response, "0030", "3955") {
			return TANChallenge{Method: method, Medium: medium}, "", true
		}
	}
	for _, s := range candidates {
		taskReference := component(s, 2, 0)
		text := component(s, 3, 0)
		// FinTS PIN/TAN defines these placeholders for initialization without
		// strong authentication. They are not a task that can be polled.
		if taskReference == "noref" && text == "nochallenge" {
			continue
		}
		// HIRMS refers to the submitted HKTAN. Some banks omit or differ in
		// HITAN's reference; a sole HITAN is unambiguous in that case.
		reference := s.Header.Reference
		if expected > 0 {
			reference = expected
		}
		matching := make([]segment, 0)
		for _, feedback := range findSegments(response, "HIRMS") {
			if reference > 0 && feedback.Header.Reference == reference {
				matching = append(matching, feedback)
			}
		}
		if hasResponseCode(matching, "3076") {
			continue
		}
		// Consors reports 0030 against HKIDN (the protected login order)
		// while HITAN references the accompanying HKTAN. Both references
		// must belong to this request; HKVVB or other orders never qualify.
		if protectedOrder > 0 && protectedOrder != reference {
			for _, feedback := range findSegments(response, "HIRMS") {
				if feedback.Header.Reference == protectedOrder && hasResponseCode([]segment{feedback}, "0030", "3955") {
					matching = append(matching, feedback)
				}
			}
		}
		code := ""
		for _, candidate := range []string{"3955", "0030"} {
			if hasResponseCode(matching, candidate) {
				code = candidate
				break
			}
		}
		if code == "" {
			if expected > 0 && hasResponseCode(response, "0030", "3955") {
				// An unresolved real challenge must never authorize business
				// requests. completeTAN reports the absent usable reference.
				return TANChallenge{Method: method, Medium: medium}, "", true
			}
			continue
		}
		// A lone placeholder is malformed, not authorization to continue.
		// completeTAN rejects a missing reference before invoking callbacks.
		if taskReference == "noref" {
			taskReference = ""
		}
		challenge := TANChallenge{
			Method: method, Medium: medium,
			Process: component(s, 0, 0), HKTANVersion: s.Header.Version,
			TaskReference: taskReference, Challenge: text,
			Decoupled: code == "3955",
		}
		if len(s.Fields) > 4 && s.Fields[4].binary {
			challenge.ChallengeHHD = append([]byte(nil), s.Fields[4].raw...)
		}
		if challenge.Challenge == "" {
			challenge.Challenge = responseText(matching, code)
		}
		decodeChallengePresentation(&challenge)
		return challenge, code, true
	}
	// Preserve a fail-closed error for a challenge lacking its HITAN data.
	if len(findSegments(response, "HITAN")) == 0 {
		for _, code := range []string{"3955", "0030"} {
			if hasResponseCode(response, code) {
				return TANChallenge{Method: method, Medium: medium}, code, true
			}
		}
	}
	return TANChallenge{}, "", false
}

func hasResponseCode(response []segment, codes ...string) bool {
	wanted := make(map[string]bool, len(codes))
	for _, code := range codes {
		wanted[code] = true
	}
	for _, kind := range []string{"HIRMG", "HIRMS"} {
		for _, s := range findSegments(response, kind) {
			for index := range s.Fields {
				if wanted[component(s, index, 0)] {
					return true
				}
			}
		}
	}
	return false
}

func responseText(response []segment, code string) string {
	for _, kind := range []string{"HIRMG", "HIRMS"} {
		for _, s := range findSegments(response, kind) {
			for index := range s.Fields {
				if component(s, index, 0) == code {
					return component(s, index, 2)
				}
			}
		}
	}
	return ""
}

func (c *Client) tanMediaLocked(ctx context.Context) ([]TANMedium, error) {
	if err := c.ensureAuthentication(ctx); err != nil {
		return nil, err
	}
	version := c.hktabVersion
	if version != 4 && version != 5 {
		version = 5
	}
	security := c.config.SecurityFunction
	if security == "" {
		security = defaultSecurity
	}
	hktanVersion := c.hktanVersion()
	useSCA := security != defaultSecurity && hktanVersion >= 6
	media, err := c.fetchTANMediaDialog(ctx, version, security, useSCA)
	if err == nil || !useSCA || ctx.Err() != nil {
		return media, err
	}
	var bankErr *BankError
	if !errors.As(err, &bankErr) {
		return nil, err
	}

	// HBCI4Java first initializes this special dialog with an HKTAN
	// referring to HKTAB. A few older endpoints reject that PSD2 flow, so it
	// retries once without the initialization SCA.
	media, fallbackErr := c.fetchTANMediaDialog(ctx, version, security, false)
	if fallbackErr == nil {
		return media, nil
	}
	return nil, fmt.Errorf(
		"HKTAN TAN-media retrieval failed (%v); fallback without initialization SCA failed: %w",
		err,
		fallbackErr,
	)
}

func (c *Client) fetchTANMediaDialog(
	ctx context.Context,
	version int,
	security string,
	useSCA bool,
) ([]TANMedium, error) {
	d := &dialog{id: "0", nextMessage: 1, systemID: c.systemID, security: security}
	initCommands := []string{
		fmt.Sprintf("HKIDN:{n}:2+%s:%s+%s+%s+1'",
			escape(c.config.CountryCode), escape(c.config.BankCode), escape(c.config.CustomerID), escape(c.systemID)),
		fmt.Sprintf("HKVVB:{n}:3+%d+%d+0+%s+%s'",
			c.bpdVersion, c.updVersion, escape(c.config.ProductID), escape(c.config.ProductVersion)),
	}
	if useSCA {
		medium := ""
		mediumRequired := containsString(
			c.config.Compatibility.TANMediumRequiredForFunctions,
			security,
		)
		if method, ok := c.tanMethods[security]; ok && method.MediumRequired {
			mediumRequired = true
		}
		if mediumRequired {
			// HBCI4Java uses the standardized "noref" placeholder here:
			// the concrete medium name cannot be supplied while HKTAB is
			// being used to discover that very name.
			medium = "noref"
		}
		initCommands = append(
			initCommands,
			buildHKTAN(c.hktanVersion(), "4", "HKTAB", "", medium),
		)
	}
	initResponse, err := c.sendCommands(ctx, d, initCommands)
	if err != nil {
		return nil, fmt.Errorf("initialize TAN-media dialog: %w", err)
	}
	d.id = responseDialogID(initResponse)
	if d.id == "" || d.id == "0" {
		return nil, failureWithResponse(errors.New("bank did not return a dialog ID for TAN-media retrieval"), "tan_media", "missing_dialog_id", initCommands, initResponse)
	}
	defer c.endDialog(ctx, d)

	followup, err := c.completeTAN(ctx, d, initResponse)
	if err != nil {
		return nil, fmt.Errorf("authenticate TAN-media dialog: %w", err)
	}

	// HKTAB is a regular order message in the initialized dialog. In
	// particular it must not share the first message with HKIDN/HKVVB;
	// Sparkasse servers reject that composition with 9110.
	mediaResponse, err := c.sendCommands(ctx, d, []string{
		fmt.Sprintf("HKTAB:{n}:%d+0+A'", version),
	})
	if err != nil {
		return nil, fmt.Errorf("fetch TAN media: %w", err)
	}
	response := append(initResponse, followup...)
	response = append(response, mediaResponse...)
	media := tanMediaFromResponse(response)
	return media, nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func tanMediaFromResponse(response []segment) []TANMedium {
	var result []TANMedium
	seen := make(map[string]bool)
	for _, s := range findSegments(response, "HITAB") {
		for _, f := range s.Fields {
			values := f.components
			if len(values) < 10 {
				continue
			}
			nameIndex := 13
			if s.Header.Version == 4 {
				nameIndex = 12
			}
			name := valuesAt(values, nameIndex)
			if name == "" {
				// Compatibility with older HITAB layouts that predate the
				// medium-class and security-function fields.
				if s.Header.Version == 4 {
					name = valuesAt(values, 9)
				} else {
					name = valuesAt(values, 10)
				}
			}
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			result = append(result, TANMedium{
				Name:             name,
				Class:            valuesAt(values, 0),
				Status:           valuesAt(values, 1),
				SecurityFunction: valuesAt(values, 2),
				MobileNumber:     firstNonEmpty(valuesAt(values, 15), valuesAt(values, 8)),
			})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// SetTANMethod selects a method previously returned by TANMethods. It discards
// the previous method's device selection and forces fresh authenticated UPD.
// Like other Client methods, do not call this from an authentication callback.
func (c *Client) SetTANMethod(securityFunction string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	method, ok := c.tanMethods[securityFunction]
	if !ok {
		return errors.New("selected TAN method was not advertised by the bank")
	}
	if len(c.allowedSecurity) > 0 && !containsString(c.allowedSecurity, securityFunction) {
		return errors.New("selected TAN method is not allowed for this user")
	}
	if c.config.SecurityFunction != securityFunction {
		c.config.TANMedium = ""
	}
	c.config.SecurityFunction = securityFunction
	c.config.HKTANVersion = method.HKTANVersion
	c.authenticatedParametersReady = false
	return nil
}

// decodeChallengePresentation independently decodes the FinTS HHD_UC matrix
// container (two-byte big-endian MIME length, MIME, two-byte image length,
// image) and CHLGUC/CHLGTEXT text envelopes. Unknown HHD formats are retained
// verbatim for a caller-provided optical TAN renderer. No bank HTML is trusted.
func decodeChallengePresentation(challenge *TANChallenge) {
	text := challenge.Challenge
	if strings.HasPrefix(text, "CHLGUC  ") && len(text) >= 12 {
		size, err := strconv.Atoi(text[8:12])
		if err == nil && size >= 0 && size <= len(text)-12 {
			encoded := text[12 : 12+size]
			if len(challenge.ChallengeHHD) == 0 {
				challenge.ChallengeHHD = []byte(encoded)
			}
			if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil && bytes.HasPrefix(decoded, []byte("\x89PNG\r\n\x1a\n")) {
				challenge.ImageMIMEType, challenge.Image = "image/png", decoded
			}
			text = text[12+size:]
		}
	}
	if strings.HasPrefix(text, "CHLGTEXT") && len(text) >= 12 {
		size, err := strconv.Atoi(text[8:12])
		if err == nil && size >= 0 && size <= len(text)-12 {
			text = text[12 : 12+size]
		}
	}
	challenge.Challenge = text
	data := challenge.ChallengeHHD
	if len(data) < 4 {
		return
	}
	mimeLength := int(binaryencoding.BigEndian.Uint16(data[:2]))
	if mimeLength > len(data)-4 {
		return
	}
	contentStart := 4 + mimeLength
	contentLength := int(binaryencoding.BigEndian.Uint16(data[2+mimeLength : contentStart]))
	if contentLength == 0 || contentLength > len(data)-contentStart {
		return
	}
	mime := string(data[2 : 2+mimeLength])
	// Reject SVG/HTML and all other active formats. The caller can display
	// these bounded raster payloads as images, but must not execute them.
	switch mime {
	case "image/png", "image/jpeg", "image/gif":
		challenge.ImageMIMEType = mime
		challenge.Image = append([]byte(nil), data[contentStart:contentStart+contentLength]...)
	}
}
