module papierleicht.de/e-rechnung-studio

go 1.27.0

require (
	github.com/BergerAPI/go-fints v0.0.0
	github.com/beevik/etree v1.8.1
	github.com/boxesandglue/baseline-pdf v1.1.34
	github.com/boxesandglue/boxesandglue v0.2.101
	github.com/boxesandglue/htmlbag v0.0.102
	github.com/google/uuid v1.6.0
	github.com/knroy/go-xml v0.0.0-00010101000000-000000000000
	github.com/shopspring/decimal v1.6.0
	github.com/speedata/pdfdisassembler v0.0.9
	github.com/wailsapp/wails/v3 v3.1.0
	github.com/zalando/go-keyring v0.2.8
	golang.org/x/net v0.62.0
	golang.org/x/sys v0.49.0
	golang.org/x/text v0.43.0
	modernc.org/sqlite v1.62.0
)

require (
	github.com/PuerkitoBio/goquery v1.13.0 // indirect
	github.com/andybalholm/brotli v1.2.8 // indirect
	github.com/andybalholm/cascadia v1.3.5 // indirect
	github.com/boxesandglue/gofpdi v1.0.26 // indirect
	github.com/boxesandglue/svgreader v0.0.8 // indirect
	github.com/boxesandglue/textshape v0.0.19 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/coder/websocket v1.8.15 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/ebitengine/purego v0.11.3 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.4 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/speedata/barcode v1.1.1 // indirect
	github.com/speedata/css v1.0.5 // indirect
	github.com/speedata/hyphenation v1.0.5 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/term v0.47.0 // indirect
	modernc.org/libc v1.80.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

replace github.com/knroy/go-xml => ./third_party/go-xml

replace github.com/BergerAPI/go-fints => ./third_party/go-fints
