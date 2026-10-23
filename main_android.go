//go:build android

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// Android loads the Go code as a shared library, so the runtime never calls
// main() by itself. Registering it here lets the native bridge start the app
// once the activity and the JNI bridge are ready.
func init() {
	application.RegisterAndroidMain(main)
}
