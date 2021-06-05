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
	"sync"
)

const (
	sendFile = "test.txt"
	times    = 1024
)

func zipHandler(w http.ResponseWriter, r *http.Request) {
	buf := new(bytes.Buffer)
	writer := zip.NewWriter(buf)
	data, err := ioutil.ReadFile(sendFile)
	if err != nil {
		log.Fatal(err)
	}
	for time := 0; time < times; time++ {
		filename := fmt.Sprintf("test/%d.txt", time)
		log.Println("start sending file", time)
		f, err := writer.Create(filename)
		if err != nil {
			log.Fatal(err)
		}
		_, err = f.Write([]byte(data))
		if err != nil {
			log.Fatal(err)
		}

	}
	err = writer.Close()
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
			readFile, err := os.Open(sendFile)
			if err != nil {
				log.Fatal(err)
			}
			buf := make([]byte, 1024)
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
			dataRead := make([]byte, 1024)
			n, err := pr.Read(dataRead)
			for times := 0; times < 1024; times++ {
				w.Write(dataRead[:n])
			}
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
		readFile, err := os.Open(sendFile)
		if err != nil {
			log.Fatal(err)
		}
		buf := make([]byte, 1024)
		for {
			n, err := readFile.Read(buf)
			for time := 0; time < times; time++ {
				f.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
	}
}

func main() {
	http.HandleFunc("/all-content", zipHandler)
	http.HandleFunc("/stream/pipe", zipHandlerUsingPipe)
	http.HandleFunc("/stream/resp", zipHandlerUsingResp)
	http.ListenAndServe(":8080", nil)
}
