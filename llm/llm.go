package llm

// #cgo CFLAGS: -Illama.cpp
// #cgo darwin,arm64 LDFLAGS: -lstdc++ -L${SRCDIR}/build/darwin/arm64/metal -lllama -framework Accelerate -framework Foundation -framework Metal -framework MetalKit -framework MetalPerformanceShaders
// #cgo darwin,amd64 LDFLAGS: -lstdc++ -L${SRCDIR}/build/darwin/x86_64/cpu -lllama
// #include "llama.h"
import "C"

// SystemInfo is an unused example of calling llama.cpp functions using CGo
func SystemInfo() string {
	return C.GoString(C.llama_print_system_info())
}
