//go:build !darwin

package inputsource

import "errors"

func Current() (Source, error) {
	return Source{}, errors.New("automatisk inndatakilde støttes foreløpig bare på macOS")
}
