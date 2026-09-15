CGO_ENABLED=0 GOOS=windows GOARCH=amd64   go build -buildvcs=false -trimpath   -ldflags="-s -w -H=windowsgui"   -o dist/screenshot-win.exe ./cmd/screenshot-win
