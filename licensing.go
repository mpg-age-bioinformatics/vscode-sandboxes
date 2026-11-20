// Package licensing carries the project and dependency notices into binaries.
package licensing

import _ "embed"

//go:embed LICENSE
var License string

//go:embed THIRD-PARTY-NOTICES.txt
var ThirdPartyNotices string

//go:embed DISCLAIMER.txt
var Disclaimer string

//go:embed APPTAINER.md
var ApptainerGuide string
