package mail

// Preset fills host, port and encryption for a common German or international
// provider. Values are the providers' published SMTP submission settings
// (checked 2026-10). Users can always change them.
type Preset struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Security string `json:"security"`
	Hint     string `json:"hint"`
}

var presets = []Preset{
	{ID: "gmail", Label: "Gmail / Google Workspace", Host: "smtp.gmail.com", Port: 587, Security: SecuritySTARTTLS,
		Hint: "Dafür brauchst du die Bestätigung in zwei Schritten und ein App-Passwort (Google-Konto → Sicherheit → App-Passwörter). In Google Workspace muss ggf. dein Administrator das erlauben."},
	{ID: "microsoft365", Label: "Microsoft 365 / Exchange Online", Host: "smtp.office365.com", Port: 587, Security: SecuritySTARTTLS,
		Hint: "Microsoft schränkt die Anmeldung per Passwort (SMTP AUTH) stark ein. Sie klappt nur, wenn sie für dein Postfach freigeschaltet ist. OAuth wird nicht unterstützt."},
	{ID: "outlook", Label: "Outlook.com / Hotmail", Host: "smtp-mail.outlook.com", Port: 587, Security: SecuritySTARTTLS,
		Hint: "Bei privaten Konten verlangt Microsoft zunehmend OAuth. Die Anmeldung per App-Passwort klappt nicht bei jedem Konto."},
	{ID: "ionos", Label: "IONOS", Host: "smtp.ionos.de", Port: 587, Security: SecuritySTARTTLS,
		Hint: "Als Benutzername gibst du deine vollständige E-Mail-Adresse ein."},
	{ID: "strato", Label: "Strato", Host: "smtp.strato.de", Port: 465, Security: SecurityTLS,
		Hint: "Als Benutzername gibst du deine vollständige E-Mail-Adresse ein."},
	{ID: "gmx", Label: "GMX", Host: "mail.gmx.net", Port: 587, Security: SecuritySTARTTLS,
		Hint: "Aktiviere in den GMX-Einstellungen unter POP3/IMAP den Zugriff über externe Programme."},
	{ID: "webde", Label: "web.de", Host: "smtp.web.de", Port: 587, Security: SecuritySTARTTLS,
		Hint: "Aktiviere in den WEB.DE-Einstellungen unter POP3/IMAP den Zugriff über externe Programme."},
	{ID: "telekom", Label: "T-Online / Telekom Mail", Host: "securesmtp.t-online.de", Port: 465, Security: SecurityTLS,
		Hint: "Nimm das separate E-Mail-Passwort aus dem Telekom-Kundencenter, nicht dein Login-Passwort."},
}

// Presets returns a copy of the built-in provider presets.
func Presets() []Preset { return append([]Preset(nil), presets...) }
