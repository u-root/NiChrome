module github.com/u-root/NiChrome

go 1.17

require (
	github.com/gorilla/mux v1.8.0
	github.com/spf13/pflag v1.0.5
	github.com/u-root/u-root v0.0.0-00010101000000-000000000000
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519
)

require (
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.11 // indirect
	golang.org/x/sys v0.0.0-20211205182925-97ca703d548d // indirect
)

replace github.com/u-root/u-root => ../u-root
