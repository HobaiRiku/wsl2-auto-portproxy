package util

import (
	"os"
)

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func CreatePathIfNotExist(path string) (bool, error) {
	pathExist, err := PathExists(path)
	if err != nil {
		return false, err
	}
	if !pathExist {
		err := os.MkdirAll(path, os.ModePerm)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}
