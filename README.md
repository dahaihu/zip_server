
最近工作中遇到一个需求，写一个动态打包`zip`文件的接口。

## 第一步，google

在遇到这个需求的时候，第一步是google，然后就看到`stackoverflow`上有一个答案，如下

```go
package main

import (
    "archive/zip"
    "bytes"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
)

func zipHandler(w http.ResponseWriter, r *http.Request) {
    filename := "randomfile.jpg"
    buf := new(bytes.Buffer)
    writer := zip.NewWriter(buf)
    data, err := ioutil.ReadFile(filename)
    if err != nil {
        log.Fatal(err)
    }
    f, err := writer.Create(filename)
    if err != nil {
        log.Fatal(err)
    }
    _, err = f.Write([]byte(data))
    if err != nil {
        log.Fatal(err)
    }
    err = writer.Close()
    if err != nil {
        log.Fatal(err)
    }
    w.Header().Set("Content-Type", "application/zip")
    w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.zip\"", filename))
    //io.Copy(w, buf)
    w.Write(buf.Bytes())
}

func main() {
    http.HandleFunc("/zip", zipHandler)
    http.ListenAndServe(":8080", nil)
}
```

这个接口看上去是没有问题的，因为仅仅打包一个jpg，是消耗不了多大的内存。但是对于大文件的话会出现如下两个问题：

1. 打包的文件中包括大文件的时候，需要等到文件一个一个读入到内存，然后写入到`zip`中，最后再写入到`http.ResponseWriter`。此时对内存的需求就比较大了。如果用户下载的`zip`文件是**2G**，那这个服务队内存的需要就最少是**2G**的。想来很多服务是没有这么奢侈的。如果这个接口同时被多个用户使用，那么服务就需要更大的内存来支持了。

2. 如果打包的文件比较大，打包过程比较长，用户的体验也是比较差的。用户在点击下载之后，看着浏览器没有响应，可能会持续的点击。这种情况下，web服务进行重复的打包，对内存的占用会进行加倍，这个时候可能会把服务整挂掉的。

## 第二步，继续  google

我觉着应该有一种边写边传输的方式，所以在搜索的关键字中加了`stream`，然后就找到一个[ruby实现](https://piotrmurach.com/articles/streaming-large-zip-files-in-rails/)。虽然不懂`ruby`，但是里面关于`http`的讲解还是可以看懂点的。关键点在于`header`中的**Content-Length**如下

1. header中**Content-Length**是表示响应长度的，浏览器会根据此字段来判断内容是否全部接受完成。如果**Content-Length**大于文件的实际长度，那么浏览器会认为下载文件失败；如果**Content-Length**小于文件的实际长度，那么浏览器会提早结束数据的接受。
2. header中**Content-Length**是可以去掉的，这个时候浏览器会一直接受请求，直到服务结束数据的传输。

好了，所以在`server`处理请求的时候，需要去掉`Header`中的**Content-Length**。

之前的数据是根据`buf`新建一个`zip.Writer`，然后在`zip.Writer`的基础上创建文件、写入文件内容，最后把此`buf`的数据传输给`http.ResponseWriter`的。现在下载不能等了，因为需要要边写入边传输。于是看起来有一个完美的答案，就是利用`pr, pw := io.Pipe()`来创建一个管道，一个用于输入，一个用于输出。代码如下

```go
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
```



## 第三步，优化

真的有必要使用`pipe`吗？一个读、一个写，为什么不能直接往`http.ResponseWriter`里面写呢？函数`zip.NewWriter`源码如下

```go
// NewWriter returns a new Writer writing a zip file to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{cw: &countWriter{w: bufio.NewWriter(w)}}
}
```

可以知道`NewWriter`接受的参数是一个接口`io.Writer`，需要实现的函数如下

```go
type Writer interface {
	Write(p []byte) (n int, err error)
}
```

而通过查看`http.ResponseWriter`的定义，可以知道，其实现了`Write(p []byte) (int, errro)`方法

```go
type ResponseWriter interface {
	Header() Header
	Write([]byte) (int, error)
	WriteHeader(statusCode int)
}
```

这样的话，我们是直接可以通过`zip.NewWriter(w)`来创建一个`writer`。这样就避免了持续往`http.ResponseWriter`写数据的过程了，因为在不断的往`zip.Writer`写入数据的时候，就会持续的往`http.ResponseWriter`写数据了。

这个时候代码就如下了

```go
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
		fmt.Println("send file path is ", sendFilePath(time))
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
```

## 总结

通过不断的加深对`http`请求以及`Writer`的理解，逐步的去掉不需要的处理逻辑，最后实现了一个算得上完美的解决方案。这种学习的过程还是挺让人开心的。<br>
本文的全部代码在`https://github.com/dahaihu/zip_server`，觉得有用的同学，可以给文章点个赞呀！！！

