package jlog

import (
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func gzipFile(filename string) {
	inputFile, err := os.Open(filename)
	if err != nil {
		return
	}
	newName := filename + gzSuffix
	outputFile, err := os.Create(newName)
	if err != nil {
		_ = inputFile.Close()
		_ = os.Rename(filename, newName)
		return
	}
	defer func() {
		_ = inputFile.Close()
		_ = outputFile.Close()
		if err != nil {
			_ = os.Rename(filename, newName)
		} else {
			fmt.Println("remove src file:", filename)
			os.Remove(filename)
		}
	}()
	gzipWriter, err := gzip.NewWriterLevel(outputFile, flate.BestCompression)
	if err != nil {
		return
	}
	defer gzipWriter.Close()
	_, err = io.Copy(gzipWriter, inputFile)
	if err != nil {
		return
	}
}

func rotateLog(s severity) {
	for i := logCount; i >= 1; i-- {
		newName := filepath.Join(logDir, processName+severityName[s]+"."+fmt.Sprintf("%d", i))
		if compress {
			newName += gzSuffix
		}
		if i == logCount {
			_ = os.Remove(newName)
			//	fmt.Println("remove", newName)
		}
		oldName := filepath.Join(logDir, processName+severityName[s]+"."+fmt.Sprintf("%d", i-1))
		if compress {
			oldName += gzSuffix
		}
		_ = os.Rename(oldName, newName)
		//fmt.Println("rename", oldName, "to", newName)
	}
	oldName := filepath.Join(logDir, processName+severityName[s]+".temp")
	newName := filepath.Join(logDir, processName+severityName[s]+".0")
	_ = os.Rename(oldName, newName)
	if compress {
		gzipFile(newName)
	}
}
