package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

func sendFilePath(time int) string {
	return fmt.Sprintf("%d.txt", time)
}

func init() {
	// generate times files, every one is 500m
	for time := 0; time < times; time++ {
		content := strings.Repeat(strconv.Itoa(time), 1024*1024*500)
		err := ioutil.WriteFile(
			sendFilePath(time),
			[]byte(content),
			0644,
		)
		if err != nil {
			panic(err)
		}
	}
	log.Println("file generated succeed")
}

const (
	times        = 10
	bufferLength = 1024 * 1024 * 10
)

func zipHandler(w http.ResponseWriter, _ *http.Request) {
	buf := new(bytes.Buffer)
	writer := zip.NewWriter(buf)
	for time := 0; time < times; time++ {
		readFile, err := os.Open(sendFilePath(time))
		if err != nil {
			log.Fatal(err)
		}
		data, err := ioutil.ReadAll(readFile)
		if err != nil {
			log.Fatal(err)
		}
		filename := fmt.Sprintf("test/%d.txt", time)
		log.Println("start sending file", time)
		f, err := writer.Create(filename)
		if err != nil {
			log.Fatal(err)
		}
		_, err = f.Write(data)
		if err != nil {
			log.Fatal(err)
		}
	}
	err := writer.Close()
	if err != nil {
		log.Fatal(err)
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"test.zip\"")
	//io.Copy(w, buf)
	w.Write(buf.Bytes())
}

func zipHandlerUsingPipe(w http.ResponseWriter, r *http.Request) {
	pr, pw := io.Pipe()
	writer := zip.NewWriter(pw)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"test.zip\"")
	w.Header().Del("Content-Length")
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer pw.Close()
		defer writer.Close()
		for time := 0; time < times; time++ {
			filename := fmt.Sprintf("test/%d.txt", time)
			log.Println("start sending file", time)
			f, err := writer.Create(filename)
			if err != nil {
				log.Fatal(err)
			}
			readFile, err := os.Open(sendFilePath(time))
			if err != nil {
				log.Fatal(err)
			}
			buf := make([]byte, bufferLength)
			for {
				n, err := readFile.Read(buf)
				f.Write(buf[:n])
				if err != nil {
					break
				}
			}
		}
	}()

	go func() {
		defer wg.Done()
		for {
			dataRead := make([]byte, bufferLength)
			n, err := pr.Read(dataRead)
			w.Write(dataRead[:n])
			if err != nil {
				return
			}
		}
	}()
	wg.Wait()
}

func zipHandlerUsingResp(w http.ResponseWriter, r *http.Request) {
	writer := zip.NewWriter(w)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"test.zip\"")
	w.Header().Del("Content-Length")
	defer writer.Close()
	for time := 0; time < times; time++ {
		filename := fmt.Sprintf("test/%d.txt", time)
		log.Println("start sending file", time)
		f, err := writer.Create(filename)
		if err != nil {
			log.Fatal(err)
		}
		log.Println("send file path is ", sendFilePath(time))
		readFile, err := os.Open(sendFilePath(time))
		if err != nil {
			log.Fatal(err)
		}
		buf := make([]byte, bufferLength)
		for {
			n, err := readFile.Read(buf)
			f.Write(buf[:n])
			if err != nil {
				break
			}
		}
		readFile.Close()
	}
}

func main() {
	http.HandleFunc("/all-content", zipHandler)
	http.HandleFunc("/stream/pipe", zipHandlerUsingPipe)
	http.HandleFunc("/stream/resp", zipHandlerUsingResp)
	_ = http.ListenAndServe(":8080", nil)
}
