package main

import (
	"math/rand"
	"time"
)

func main() {
	pubrate := 100.0
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	delay := r.ExpFloat64() / pubrate
	time.Sleep(time.Duration(delay*1000000) * time.Microsecond)
}
