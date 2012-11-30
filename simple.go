package main
import (
    "fmt"; "runtime"
)
func main() {
    fmt.Printf("numGoroutine = %d\n", runtime.NumGoroutine())
}
