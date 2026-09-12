module github.com/AaronFeledy/claude-usage-widget/packaging/headroom-manager

go 1.25

require (
	golang.org/x/sys v0.21.0
	golang.org/x/term v0.18.0
)

require github.com/AaronFeledy/claude-usage-widget/server v0.0.0

require (
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/AaronFeledy/claude-usage-widget/server => ../../server
