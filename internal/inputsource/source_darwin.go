package inputsource

import (
	"bytes"
	"errors"
	"sync"

	"github.com/ebitengine/purego"
)

const utf8Encoding = 0x08000100

type carbonAPI struct {
	copyCurrent func() uintptr
	property    func(uintptr, uintptr) uintptr
	newString   func(uintptr, string, uint32) uintptr
	getString   func(uintptr, *byte, int, uint32) bool
	release     func(uintptr)
	id, name    uintptr
}

// Loading frameworks dynamically keeps the existing CGO_ENABLED=0 macOS build.
var loadAPI = sync.OnceValues(func() (*carbonAPI, error) {
	carbon, err := purego.Dlopen("/System/Library/Frameworks/Carbon.framework/Carbon", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return nil, err
	}
	core, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return nil, err
	}
	a := &carbonAPI{}
	purego.RegisterLibFunc(&a.copyCurrent, carbon, "TISCopyCurrentKeyboardInputSource")
	purego.RegisterLibFunc(&a.property, carbon, "TISGetInputSourceProperty")
	purego.RegisterLibFunc(&a.newString, core, "CFStringCreateWithCString")
	purego.RegisterLibFunc(&a.getString, core, "CFStringGetCString")
	purego.RegisterLibFunc(&a.release, core, "CFRelease")
	// These immutable CFString keys live for the lifetime of the loaded API.
	a.id = a.newString(0, "TISPropertyInputSourceID", utf8Encoding)
	a.name = a.newString(0, "TISPropertyLocalizedName", utf8Encoding)
	if a.id == 0 || a.name == 0 {
		return nil, errors.New("kunne ikke opprette nøkler for inndatakilde")
	}
	return a, nil
})

func Current() (Source, error) {
	a, err := loadAPI()
	if err != nil {
		return Source{}, err
	}
	source := a.copyCurrent()
	if source == 0 {
		return Source{}, errors.New("ingen aktiv inndatakilde")
	}
	defer a.release(source)
	read := func(key uintptr) string {
		value := a.property(source, key)
		if value == 0 {
			return ""
		}
		var buffer [4096]byte
		if !a.getString(value, &buffer[0], len(buffer), utf8Encoding) {
			return ""
		}
		return string(bytes.TrimRight(buffer[:], "\x00"))
	}
	result := Source{ID: read(a.id), Name: read(a.name)}
	if result.ID == "" {
		return Source{}, errors.New("kunne ikke lese aktiv inndatakilde")
	}
	return result, nil
}
