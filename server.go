package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"os/signal"
	"os/user"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	dir               string             // Root directory for file server
	port              string             // Port on which file server should run
	version           bool               // Display version
	help              bool               // Display help
	htmlHeadTemplate  *template.Template // Template for html begin
	tableItemTemplate *template.Template // Template for table item
	fileTypes         = map[string]string{
		".jpg":  "image",
		".jpeg": "image",
		".png":  "image",
		".bmp":  "image",
		".gif":  "image",
		".mp3":  "audio",
		".wav":  "audio",
		".wma":  "audio",
		".mp4":  "video",
		".mpg":  "video",
		".mpeg": "video",
		".avi":  "video",
		".mkv":  "video",
		".pdf":  "document",
		".doc":  "document",
		".docx": "document",
		".text": "document",
		".ppt":  "document",
		".pptx": "document",
		".xml":  "document",
		".html": "web",
		".htm":  "web",
		".css":  "web",
		".js":   "web",
		".c":    "develop",
		".cpp":  "develop",
		".java": "develop",
		".cs":   "develop",
		".go":   "develop",
		".sh":   "develop",
		".rb":   "develop",
		".php":  "develop",
		".py":   "develop",
	}

	sizes = map[int]string{
		0: " B",
		1: " KB",
		2: " MB",
		3: " GB",
		4: " TB",
	}
)

var htmlReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	// "&#34;" is shorter than "&quot;".
	`"`, "&#34;",
	// "&#39;" is shorter than "&apos;" and apos was not in HTML until HTML5.
	"'", "&#39;",
)

// countingWriter counts how many bytes have been written to it.
type countingWriter int64

func (w *countingWriter) Write(p []byte) (n int, err error) {
	*w += countingWriter(len(p))
	return len(p), nil
}

type httpRange struct {
	start, length int64
}

func (r httpRange) contentRange(size int64) string {
	return fmt.Sprintf("bytes %d-%d/%d", r.start, r.start+r.length-1, size)
}

func (r httpRange) mimeHeader(contentType string, size int64) textproto.MIMEHeader {
	return textproto.MIMEHeader{
		"Content-Range": {r.contentRange(size)},
		"Content-Type":  {contentType},
	}
}

type item struct {
	Icon         string
	Name         string
	Path         string
	LastModified string
	Size         string
	Target       string
}

const DATEFORMAT = "2006-01-02 15:04:05"
const sniffLen = 512

const HTMLDOCUMENTBEGIN = `
<html>
	<head>
		<title> {{.}} </title>
		<style>
			body {margin: 0; padding-top: 10px; background-color: #edece4; font-family: Tahoma, Geneva, sans-serif; color: #4d4d4d}
			.contents{margin: 0 auto;}
			a:link{color: #333333; text-decoration: none;}
			a:visited{color: #333333; text-decoration: none;}
			a:active{color: #333333; text-decoration: none;}
			table {margin: 0 auto; background-color: #fff; padding: 40px; border: solid 1px #d9d8d4;}
			tr:hover {background-color: rgba(243, 243, 243, 0.85);}
			td {padding: 3px 20px 3px 0;}
			th {text-align: left; border-bottom: 1px solid #4d4d4d;}
			.footer {background-color: #fff; border-top: solid 1px #d9d8d4; border-bottom: solid 1px #d9d8d4; height: 35px; padding-top: 15px; margin-top: 10px; margin-bottom: 10px; text-align: center;}
			.homeButton {position: fixed; border: solid 1px #d9d8d4; background-color: #fff;}
			.backButton {top: 65px; position: fixed; border: solid 1px #d9d8d4; background-color: #fff;}
			.icon {width: 16px; height: 16px; background-repeat: no-repeat;}
			.icons {padding: 2px 2px 2px 0;}
			.button {width: 32px; height: 32px; background-repeat: no-repeat;}
			.image { background-image: url(data:image/png;base64,R0lGODlhEAAQAPcAAEKU50Kl91JCQmNCKWNjY2sxIWtjtWtzpXOEtXOl53O173uMxnuUxnul3oQAhIRzpYSczoStzoSt3oxaMYyEtYyUtZSEpZSUxpSlzpTG95yl1qWElKWUlKW956XO960xKa1rIa2Ura21/63W/7UxELVaMbVzQrWEa7WclLXW/72Me73O/8aclMbW/8bn/84xGM6Uc9ZCGNacc9alc9be59bv/95KId6UWt61hN7n7+dSIeeUUufOhOfn5++EOe/Ge+/v9+/3//e9Y/fGc/9rMf9zMf+EOf+EUv+MQv+UQv+UY/+tUv+1Wv/GY//We//enP/ne//nhP///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////yH+EkR1c3RpbiBGcmllc2VuaGFobgAh+QQBAAAOACwAAAAAEAAQAAAIwgAlCBw4kICDgwglSFnIUIoEBQYROlDYcGGAAAAiHqTYMEiLFiIQaFSohAgRJEuaBKlRY8VIKUdKFEnC5MmQJRhcJpTyoQCJFzF06LABQefGIDsmgPBhRIgRHxGMTgQyw8SAGzucOLkhQSqEHixUCDghg8cPGA2kMsgR4oGBDTyiQEHRwIPGBTkuULBggQOOHxYS2EWIgEaNESM8eMjAeEQHjYVruJicojLixwgPaNisAcOFzxcqUNDogIDp06hNIwwIADs=);}
			.video { background-image: url(data:image/png;base64,R0lGODlhEAAQAMQfAN/f3zw8PEpKSkBAQBQUFMPDw+bm5hwcHNvb20tLS8bGxlhYWKSkpDMzM2traxgYGERERLKyslNTUz4+PkhISBkZGXZ2dkZGRikpKTs7Ozg4OMzMzP///wsLCwAAAAAAACH5BAEAAB8ALAAAAAAQABAAAAVi4CeOJMBxAFmenncWK9eyLgeLs1x7A6CQOV5ks1HhMAwiwmAceTrQqLTjGXUcFsEiEDgIIIJHxxqQTBKaTIMyuFTGImhrTodaqfQ6/CPPz+1xeH5Pex0EU1KHTohTVU2PIiEAOw==);}
			.audio { background-image: url(data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGXRFWHRTb2Z0d2FyZQBBZG9iZSBJbWFnZVJlYWR5ccllPAAAAqFJREFUeNp0Us9PE1EQnre77W5Zty1SWlD5YY1RxJig0YSTHIheTQz1P/BC5OBFE6PEK4kJRogelQux4sEE46UXLhghGIWQaGJYEVqbbbW4LVu23d3nvN1CCwkv+bL75s18880PQimFRAoOHlr7JhEziPcI46BTchCAg0NOKrU0sbKi9mtaIek4Tg5NrxFDiKZGP+Ewgnj82B38jCwvr/XzPDcUiYRvxmLhRCQSAo7j5vBtYI/AMOi+YA51+XwCbW0NJePx9plCofSgUCjeZWT4fG1w8NLori9hPeh7Xj1AQKiqfn1MCEkEg3JPNNpsRKPh2UDAv9uPbRbLeuASXHhmQC7PNSqgjrNNTPMnu/YiEkg2pChyT0tLELq7293kb6/XmtgcsqGri4cqCmEwTYBKRQZCZPa8ihjFROd0vXReVTP3slkbNM2ul3B1qgg+KQA7Ozz8zlJAEyqghDno/77sK0+SToIoypQQIGv3xf1TCAQIdHYQMMoAjuPZgsEzsLnxvaamFdU1oTqWhNbHSKkz4NjWGDbvoiBwvKIQe2+V4AiIGEj5MGh5bwVYLCqsE+h/temOeFvb0YgPy6iClrVUTOk6lEu6p05yoPOEBYYp1QgaFimfTctKswL6loHQQRDEp7zgQycbzLIBflFynX2CA2HJws4JUKvAm8LWH+3R8sf54urSYjqX2RyxbXsiPfsEfr0bc52QaMCyKgu2VbWoXaGEVm2eWHUFjm2PVz6/GTd5Hky/COn5qfpOxM6C1NU3fbr3VFvsuAxGyYSN9Zy6/WMB4NYNb4zEq5dnKhH+Gti/+xC8PfOt9/IVhV20TAZ21pYebr4afoGx+cYxMgLJbbuHwG6JpdXU5GJJHxYEoUjWP70sz01+QHMLa99/AQYAohQjWPbGXSYAAAAASUVORK5CYII=);}
			.document { background-image: url(data:image/png;base64,R0lGODlhEAAQAKIFAIWFhYSEhAAAAMbGxv///////wAAAAAAACH5BAEAAAUALAAAAAAQABAAAAM9WBXcrnCRSWcQUY7NSQAYFFSVYIYapxIDOpJVK7JqJysvPN1pPbAuHYU38m2AMyESR/MtJUqiUeU6Wa+FBAA7);}
			.web { background-image: url(data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGXRFWHRTb2Z0d2FyZQBBZG9iZSBJbWFnZVJlYWR5ccllPAAAATdJREFUeNpifOqtlsDAwDCfgQwgvfUWIxOQfsBAAWCiQO8BEMECxBdgIvypVQzc/vF4db2tjGX4efkUnM8C9McHYDiAOf++fgbTv+9dZ/gPZaODfwjxDzAXYICvmxYyfNuznpAXLiIbAPKGwa/LJ4FUDgOXcxADs5gMVl2fl01G4bMgOwcG2HTNwBgd/H31FNmABxhe+H3vBjiQsAHh9sUMf18+RRZCMeAgEDv8+/oJJYThcc3NR3w6kNpyk0F83j4GdqgXmMWlGThdAsHsP69wu+ADIpo+AQNQGuxkjDBA8gIw+h+gxwIYvM4LYOCwcAG7AESDDPxxYi/Dj+N7GH5h8R4LNlu+blwIxnjABfQwADlnAZEZC6R5IozDiC4LTNYGoBgBYn8oDQqfDdCY2gBK+sjqAQIMACfPddVHcozkAAAAAElFTkSuQmCC);}
			.develop { background-image: url(data:image/png;base64,R0lGODlhEAAQALMAAI6Ojh4eHvv7+/b29vz8/Pn5+fj4+Pr6+iUlJUBAQHZ2dv///wAAAAAAAAAAAAAAACH5BAAAAAAALAAAAAAQABAAAARKcKVJK1gYp8z1zVvHJZ+EKeiiqCipnWq8rmE4y3F9pnOqi51fb2gq3o5FG+4mZBF/QNDLKfNpCAJF9qAoKAyKQS0RQJjP5sBkEQEAOw==);}
			.directory { background-image: url(data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAMAAAAoLQ9TAAABX1BMVEX39/dra2tsbGxtbW1ubm57e3t+fn5/f3+ua1Cua1G2cVO4dFS7eFW9e1a6fVe/f1e7f1i9g1m/hFnBgljDhVnFiFnGiVrHjFrDiF7HjV/IjFrIjFvJjlvKkFzLkVzMk13NlF3PlV3Oll7Pl17Rml/SnV/Pmmbfl2HTnWDUn2DUoGHVoWHWo2TZqGXZqWfRoGrToGvXqW/0rWn0t2z5tWv+uW3+vG7hrXT6zHitra2vr6+wsLCysrLJrZnLtZ/ivYbhvpDXvKrZvqn90oP33IP52oH/1orlxJPjw5nu15z45Y385pn56p3dxavjxqDjyKr/76j7763//aP05rry47z88LD89L3//77MzMzU1NTW1tbd3d399sf/+Mf48cv9+Mz9+c727dL9+dn//9v+/t3l5eXn5+fo6Ojp6en16+T48Ob9+uX//OX//uX+/up9W1D8+PX4+Pj5+fn///j///+gwhWcAAAAAXRSTlMAQObYZgAAAAFiS0dEAIgFHUgAAAAJcEhZcwAADsMAAA7DAcdvqGQAAAAHdElNRQfVAxgNNRYCF8YoAAAAr0lEQVR42mNgIAwcdDT1fdMQfD/37LxEA7touIBhXEpKSoiajLiYhCNYnVZ8AhQEC1imAgVUwzz1tDVUlORlpcUEbYFqVAJ1s6C6czL5rNIYlH20i5NiwMA/l5c1kkHBSyM5FAI8YnmYIhjkXFXCvSHAPoCbMYJBxkIpyBkCzN24gAKSxoouZqYgYKJuxAHU4iQkKiUmIszPy8PNxZnPFsWQbs3GwgwFLOw2GRh+BQApPielSWe5LQAAAABJRU5ErkJggg==);}
			.home { background-image: url(data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAYAAABzenr0AAAACXBIWXMAAAsTAAALEwEAmpwYAAAAIGNIUk0AAHolAACAgwAA+f8AAIDpAAB1MAAA6mAAADqYAAAXb5JfxUYAAAGQSURBVHja7JbRccIwDIa/cAyQbsAIyBMkG8AEwAQtG7QTdIRkA7oBTBBnA7pBswF9ke9cn0NICOV6RU/Eh/L9kazfTk6nE/eMCXeOqfuRJEnvZBEzB3bA0tqq7pPrKj+4AgrfAzNgr8+9I3FK+lRAxCyAAki95QbIra1qEfPqrZfWVp9tFegtQMSsFR6LBsgB663l1laHUVrQAn9TMFqR/U2mIAJvgCVQ6lf7IsYVIGKKCDxX2BGYByLGE6DwdQS+8kQVZ0SkgwVE4DUgwDPwEvzdF+F7QnFuRKNTIGJSNZgsgOfAeyAqjA3woZtxHo5o5xgq3E928KXCFxe0tlNEdAxb4G6X7y6Eu3YsgnakMcecePBZC3wbWR9NxNRLcL7uG0w5EO6LQEW497gqP4Ut2ER6aK+AxyrhLHkbnQJ1O6ytShHz1dfVuixFD6rM2urQeRiJmLGvSj8OpavvA6PfiAZEHbHd7DcFbMNzfkjb7t6Ch4CHgD/tAysRk91SwKEjdxacnl05zdkr2b/dA98DACBbq3XxN6XKAAAAAElFTkSuQmCC);}
			.file { background-image: url(data:image/png;base64,R0lGODlhEAAQANUvAH5+fuXl6NHR1f39/f///+Hh5PPy9O7u8Orp7NjX3N3c4Pb2987N083N0tXU2Onp7Nzc4PLy9NXU2djY3OHh4/r6+/f29+/u8Pb3+NHQ1c7N0vr5+/r6+vb2+NnX3Pn5++np7fn6+u7t8fPz9OHg5Orq7NXT2NnX3dTU2e7u8eTk5NTU2N3d4N3c39TT2dbr9QAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACH5BAEAAC8ALAAAAAAQABAAAAaLwNcLQCwahcghYclcEpPDgXQqBagASUDoU+F0N5ujEGDBLMyLdMfSwY4jEYPcMIrL3cND6nC48A8ifngACCUgCAgPiogIgwGPkJEBgyQFFAWYl5gFgwoKLRAsnhAQnoMJCScJE6isHhODDg4SDisSLg4oJhKDGQK/wMAZgwwMDcUaDQ0axoNGz0UvQQA7);}
			.back { background-image: url(data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAQAAADZc7J/AAAAAmJLR0QA/4ePzL8AAAAJcEhZcwAAAEgAAABIAEbJaz4AAAAJdnBBZwAAACAAAAAgAIf6nJ0AAAGdSURBVEjHpdXfThNBFIDx3+wuSMUKKrGmYKISfAffP/EZBBMuGhoCIRCl/O/ueNG1F3RmoeFcTSZzvvPt2TO7vDDCM04U81WjWRYQ/PBeg+DBT5PHB4rOZHYMFCqlNSOTxYJVR3q0Yq/VLp07TPl2GfDFhlpA8Ns94vMBUc+uujU5NU6l5wEBe3rz9v0S0w0vMunRps9t/crIuZCqnwNE7FnVoHDtIK2fAwQMDE0FUenAbX5eqszu91a5cmaU008bBNHQlqkgaOyrdcQi4P/4zNp35LSr/iIg4Ju3alHp1r4noniUHvV9baWDQ1fd9fNvYRaNJ6NcSL6z4mM7gRuO3Xdf+TKx99fQKhqvVI6XBQS1xlAtaGy4cNWFSBkEE1vW2+/Aa+OuNqYBjTvb7arvxkXeoczsT2y2H5No09h0OUDAtW0B0ZrCSc4hZxDc6Pkwb+WZmzQiB4BLOyqz29FzlG5lHhA8KA3UglrftT8phy4Alz5Za6fyjbFmEdH1CEGtti0qROumzpYDzPrwTk8tqvWduE+JdscTP9cXxz8u5YF32IlaaQAAACV0RVh0ZGF0ZTpjcmVhdGUAMjAxMS0wNy0wMVQxMzoxMDoyNS0wNzowMDPfgNMAAAAldEVYdGRhdGU6bW9kaWZ5ADIwMTEtMDctMDFUMTM6MTA6MjUtMDc6MDBCgjhvAAAAGXRFWHRTb2Z0d2FyZQBBZG9iZSBJbWFnZVJlYWR5ccllPAAAAABJRU5ErkJggg==);}
		</style>
	</head>
	<body><div class = 'contents'>`

const HTMLDOCUMENTEND = `
	</body>
</html>`

const TABLEBEGIN = `
<table>
	<thead>
		<th></th>
		<th>Name</th>
		<th>Size</th>
		<th>Last Modified</th>
	</thead>`

const TABLEEND = `
</table>`

const ITEM = `
	<tr>
		<td class = "icons"><div class="{{.Icon}}"></div></td>
		<td><a href="{{.Path}}" target="{{.Target}}">{{.Name}}</a></td>
		<td>{{.Size}}</td>
		<td>{{.LastModified}}</td>
	</tr>`

const (
	VERSION = "1.0"
	NAME    = "Fileserver"
	COMMAND = "fileserver"
)

const NOTFOUND = `
<html>
	<head>
		<title>404 | Not Found</title>
		<style>
		body {margin: 0; padding-top: 10px; background-color: #edece4; font-family: Tahoma, Geneva, sans-serif; color: #4d4d4d}
		.contents {margin: 0 auto; padding: 40px 80px; text-align: center; background-color: #fff; border: solid 1px #d9d8d4; width: 400px;}
		</style>
	</head>
	<body>
        <div class = "contents">
        	<h1>404</h1>
        	<h2>Not Found</h2>
        </div>
	</body>
</html>
`

func init() {
	flag.StringVar(&dir, "d", "/sdcard/", "The root directory for the file server.")
	flag.StringVar(&dir, "directory", "/sdcard/", "The root directory for the file server.")
	flag.StringVar(&port, "p", "4545", "The port on which the file server should run.")
	flag.StringVar(&port, "port", "4545", "The port on which the file server should run.")
	flag.BoolVar(&version, "v", false, "Prints the version number.")
	flag.BoolVar(&version, "version", false, "Prints the version number.")
	flag.BoolVar(&help, "h", false, "Prints the version number.")
	flag.BoolVar(&help, "help", false, "Prints the version number.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", COMMAND)
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "\t-d, -directory  Directory   The root directory for the file server.\n")
		fmt.Fprintf(os.Stderr, "\t-p, -port       Port        The port on which the file server should run.\n")
		fmt.Fprintf(os.Stderr, "\t-v, -version    Version     Prints the version number.\n")
		fmt.Fprintf(os.Stderr, "\t-h, -help       Help        Show this help.\n")
	}
	htmlHeadTemplate = template.Must(template.New("htmlStart").Parse(HTMLDOCUMENTBEGIN))
	tableItemTemplate = template.Must(template.New("tableItem").Parse(ITEM))
}

func showVersion() {
	fmt.Println("\n", NAME, VERSION)
	fmt.Println("This is a free software and comes with NO warranty.\n")
}

/*
因为http.ResponseWriter是一个接口
状态码保存在http.response私有结构体中
故此使用反射获取结构体的status字段值
会影响性能
*/
func GetStatusCode(w http.ResponseWriter) int64 {
	var status int64 = -1

	ptr := reflect.ValueOf(w)
	kind := ptr.Kind()
	if kind == reflect.Ptr {
		val := ptr.Elem()
		if val.Kind() == reflect.Struct {
			field := val.FieldByName("status")
			if field.Kind() == reflect.Int {
				status = field.Int()
			}
		}
	}
	return status
}

// 记录每个HTTP请求
func HTTPLog(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
		status_code := GetStatusCode(w)
		if r.Method != "HEAD" && r.ContentLength > 0 {
			log.Printf("%s %s %d %s %s %d", r.RemoteAddr, r.Proto, status_code, r.Method, r.URL, r.ContentLength)
		} else {
			log.Printf("%s %s %d %s %s", r.RemoteAddr, r.Proto, status_code, r.Method, r.URL)
		}
	})
}

type fileServerHandler struct {
	root http.FileSystem
}

func formatSize(size int64) string {
	sizef := float64(size)
	var i = 0
	for sizef >= 1024 && i <= 4 {
		sizef /= 1024
		i++
	}
	return strconv.FormatFloat(sizef, 'f', 2, 64) + sizes[i]
}

func (f *fileServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upath := r.URL.Path
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
		r.URL.Path = upath
	}
	serveFile(w, r, f.root, path.Clean(upath), true)
}

func serveFile(w http.ResponseWriter, r *http.Request, fs http.FileSystem, name string, redirect bool) {
	f, err := fs.Open(name)
	if err != nil {
		// TODO expose actual error?
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	d, err1 := f.Stat()
	if err1 != nil {
		// TODO expose actual error?
		http.NotFound(w, r)
		return
	}

	if redirect {
		// redirect to canonical path: / at end of directory url
		// r.URL.Path always begins with /
		url := r.URL.Path
		if d.IsDir() {
			if url[len(url)-1] != '/' {
				localRedirect(w, r, path.Base(url)+"/")
				return
			}
		} else {
			if url[len(url)-1] == '/' {
				localRedirect(w, r, "../"+path.Base(url))
				return
			}
		}
	}

	// Still a directory? (we didn't find an index.html file)
	if d.IsDir() {
		/*
			if checkLastModified(w, r, d.ModTime()) {
				return
			}
		*/

		htmlHeadTemplate.Execute(w, d.Name())
		fmt.Fprintf(w, "<a class = \"homeButton\" href=\"/\" style = 'padding: 8.5px; margin-right: 10px;'><div class=\"home button\"></div></a><a class = \"backButton\" href=\"../\" style = 'padding: 8.5px; margin-right: 10px;'><div class=\"back button\"></div></a>")
		var folders bytes.Buffer
		var files bytes.Buffer
		fmt.Fprintf(w, TABLEBEGIN)
		for {
			dirs, err := f.Readdir(100)
			if err != nil || len(dirs) == 0 {
				break
			}
			for _, d := range dirs {
				name := d.Name()
				if strings.HasPrefix(name, ".") { //TODO: Find a way to discard hidden files
					continue
				}
				if d.IsDir() {
					path := urlEscape(name) + "/"
					tableItemTemplate.Execute(&folders, item{Icon: "directory icon", Name: name, Path: path, LastModified: d.ModTime().Format(DATEFORMAT), Size: "-", Target: "_self"})
				} else {
					var image string
					if fileType, found := fileTypes[strings.ToLower(filepath.Ext(d.Name()))]; found {
						image = fileType + " icon"
					} else {
						image = "file icon"
					}
					tableItemTemplate.Execute(&files, item{Icon: image, Name: name, Path: urlEscape(name), LastModified: d.ModTime().Format(DATEFORMAT), Size: formatSize(d.Size()), Target: "_blank"})
				}
			}
		}
		fmt.Fprint(w, folders.String())
		fmt.Fprint(w, files.String())
		fmt.Fprint(w, TABLEEND)
		fmt.Fprintf(w, "</div><div class='footer'>\n")
		fmt.Fprintf(w, "<span style='font-family: \"Times New Roman\"; color: #2c2c2c; font-style:italic; font-size:14;'>Powered by Helix FileServer v%s</span>\n", VERSION)
		fmt.Fprintf(w, "</div>")
		fmt.Fprintf(w, HTMLDOCUMENTEND)

		return
	}

	// serveContent will check modification time
	sizeFunc := func() (int64, error) { return d.Size(), nil }
	serveContent(w, r, d.Name(), d.ModTime(), sizeFunc, f)
}

func serveContent(w http.ResponseWriter, r *http.Request, name string, modtime time.Time, sizeFunc func() (int64, error), content io.ReadSeeker) {
	if checkLastModified(w, r, modtime) {
		return
	}
	rangeReq, done := checkETag(w, r, modtime)
	if done {
		return
	}

	code := http.StatusOK

	// If Content-Type isn't set, use the file's extension to find it, but
	// if the Content-Type is unset explicitly, do not sniff the type.
	ctypes, haveType := w.Header()["Content-Type"]
	var ctype string
	if !haveType {
		ctype = mime.TypeByExtension(filepath.Ext(name))
		if ctype == "" {
			// read a chunk to decide between utf-8 text and binary
			var buf [sniffLen]byte
			n, _ := io.ReadFull(content, buf[:])
			ctype = http.DetectContentType(buf[:n])
			_, err := content.Seek(0, os.SEEK_SET) // rewind to output whole file
			if err != nil {
				http.Error(w, "seeker can't seek", http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", ctype)
	} else if len(ctypes) > 0 {
		ctype = ctypes[0]
	}

	size, err := sizeFunc()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// handle Content-Range header.
	sendSize := size
	var sendContent io.Reader = content
	if size >= 0 {
		ranges, err := parseRange(rangeReq, size)
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if sumRangesSize(ranges) > size {
			// The total number of bytes in all the ranges
			// is larger than the size of the file by
			// itself, so this is probably an attack, or a
			// dumb client.  Ignore the range request.
			ranges = nil
		}
		switch {
		case len(ranges) == 1:
			// RFC 2616, Section 14.16:
			// "When an HTTP message includes the content of a single
			// range (for example, a response to a request for a
			// single range, or to a request for a set of ranges
			// that overlap without any holes), this content is
			// transmitted with a Content-Range header, and a
			// Content-Length header showing the number of bytes
			// actually transferred.
			// ...
			// A response to a request for a single range MUST NOT
			// be sent using the multipart/byteranges media type."
			ra := ranges[0]
			if _, err := content.Seek(ra.start, os.SEEK_SET); err != nil {
				http.Error(w, err.Error(), http.StatusRequestedRangeNotSatisfiable)
				return
			}
			sendSize = ra.length
			code = http.StatusPartialContent
			w.Header().Set("Content-Range", ra.contentRange(size))
		case len(ranges) > 1:
			sendSize = rangesMIMESize(ranges, ctype, size)
			code = http.StatusPartialContent

			pr, pw := io.Pipe()
			mw := multipart.NewWriter(pw)
			w.Header().Set("Content-Type", "multipart/byteranges; boundary="+mw.Boundary())
			sendContent = pr
			defer pr.Close() // cause writing goroutine to fail and exit if CopyN doesn't finish.
			go func() {
				for _, ra := range ranges {
					part, err := mw.CreatePart(ra.mimeHeader(ctype, size))
					if err != nil {
						pw.CloseWithError(err)
						return
					}
					if _, err := content.Seek(ra.start, os.SEEK_SET); err != nil {
						pw.CloseWithError(err)
						return
					}
					if _, err := io.CopyN(part, content, ra.length); err != nil {
						pw.CloseWithError(err)
						return
					}
				}
				mw.Close()
				pw.Close()
			}()
		}

		w.Header().Set("Accept-Ranges", "bytes")
		if w.Header().Get("Content-Encoding") == "" {
			w.Header().Set("Content-Length", strconv.FormatInt(sendSize, 10))
		}
	}

	w.WriteHeader(code)

	if r.Method != "HEAD" {
		io.CopyN(w, sendContent, sendSize)
	}
}

func localRedirect(w http.ResponseWriter, r *http.Request, newPath string) {
	if q := r.URL.RawQuery; q != "" {
		newPath += "?" + q
	}
	w.Header().Set("Location", newPath)
	w.WriteHeader(http.StatusMovedPermanently)
}

// parseRange parses a Range header string as per RFC 2616.
func parseRange(s string, size int64) ([]httpRange, error) {
	if s == "" {
		return nil, nil // header not present
	}
	const b = "bytes="
	if !strings.HasPrefix(s, b) {
		return nil, errors.New("invalid range")
	}
	var ranges []httpRange
	for _, ra := range strings.Split(s[len(b):], ",") {
		ra = strings.TrimSpace(ra)
		if ra == "" {
			continue
		}
		i := strings.Index(ra, "-")
		if i < 0 {
			return nil, errors.New("invalid range")
		}
		start, end := strings.TrimSpace(ra[:i]), strings.TrimSpace(ra[i+1:])
		var r httpRange
		if start == "" {
			// If no start is specified, end specifies the
			// range start relative to the end of the file.
			i, err := strconv.ParseInt(end, 10, 64)
			if err != nil {
				return nil, errors.New("invalid range")
			}
			if i > size {
				i = size
			}
			r.start = size - i
			r.length = size - r.start
		} else {
			i, err := strconv.ParseInt(start, 10, 64)
			if err != nil || i > size || i < 0 {
				return nil, errors.New("invalid range")
			}
			r.start = i
			if end == "" {
				// If no end is specified, range extends to end of the file.
				r.length = size - r.start
			} else {
				i, err := strconv.ParseInt(end, 10, 64)
				if err != nil || r.start > i {
					return nil, errors.New("invalid range")
				}
				if i >= size {
					i = size - 1
				}
				r.length = i - r.start + 1
			}
		}
		ranges = append(ranges, r)
	}
	return ranges, nil
}

// checkETag implements If-None-Match and If-Range checks.
//
// The ETag or modtime must have been previously set in the
// ResponseWriter's headers.  The modtime is only compared at second
// granularity and may be the zero value to mean unknown.
//
// The return value is the effective request "Range" header to use and
// whether this request is now considered done.
func checkETag(w http.ResponseWriter, r *http.Request, modtime time.Time) (rangeReq string, done bool) {
	etag := w.Header().Get("Etag")
	rangeReq = r.Header.Get("Range")

	// Invalidate the range request if the entity doesn't match the one
	// the client was expecting.
	// "If-Range: version" means "ignore the Range: header unless version matches the
	// current file."
	// We only support ETag versions.
	// The caller must have set the ETag on the response already.
	if ir := r.Header.Get("If-Range"); ir != "" && ir != etag {
		// The If-Range value is typically the ETag value, but it may also be
		// the modtime date. See golang.org/issue/8367.
		timeMatches := false
		if !modtime.IsZero() {
			if t, err := http.ParseTime(ir); err == nil && t.Unix() == modtime.Unix() {
				timeMatches = true
			}
		}
		if !timeMatches {
			rangeReq = ""
		}
	}

	if inm := r.Header.Get("If-None-Match"); inm != "" {
		// Must know ETag.
		if etag == "" {
			return rangeReq, false
		}

		// TODO(bradfitz): non-GET/HEAD requests require more work:
		// sending a different status code on matches, and
		// also can't use weak cache validators (those with a "W/
		// prefix).  But most users of ServeContent will be using
		// it on GET or HEAD, so only support those for now.
		if r.Method != "GET" && r.Method != "HEAD" {
			return rangeReq, false
		}

		// TODO(bradfitz): deal with comma-separated or multiple-valued
		// list of If-None-match values.  For now just handle the common
		// case of a single item.
		if inm == etag || inm == "*" {
			h := w.Header()
			delete(h, "Content-Type")
			delete(h, "Content-Length")
			w.WriteHeader(http.StatusNotModified)
			return "", true
		}
	}
	return rangeReq, false
}

// modtime is the modification time of the resource to be served, or IsZero().
// return value is whether this request is now complete.
func checkLastModified(w http.ResponseWriter, r *http.Request, modtime time.Time) bool {
	if modtime.IsZero() {
		return false
	}

	// The Date-Modified header truncates sub-second precision, so
	// use mtime < t+1s instead of mtime <= t to check for unmodified.
	if t, err := time.Parse(http.TimeFormat, r.Header.Get("If-Modified-Since")); err == nil && modtime.Before(t.Add(1*time.Second)) {
		h := w.Header()
		delete(h, "Content-Type")
		delete(h, "Content-Length")
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	w.Header().Set("Last-Modified", modtime.UTC().Format(http.TimeFormat))
	return false
}

func sumRangesSize(ranges []httpRange) (size int64) {
	for _, ra := range ranges {
		size += ra.length
	}
	return
}

func rangesMIMESize(ranges []httpRange, contentType string, contentSize int64) (encSize int64) {
	var w countingWriter
	mw := multipart.NewWriter(&w)
	for _, ra := range ranges {
		mw.CreatePart(ra.mimeHeader(contentType, contentSize))
		encSize += ra.length
	}
	mw.Close()
	encSize += int64(w)
	return
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	go handleExit() // goroutine to handle ctrl + c
	flag.Parse()    // parse command line arguments

	if version {
		showVersion()
		os.Exit(0)
	}

	if help {
		flag.Usage()
		os.Exit(0)
	}

	_, fileErr := os.Stat(dir)
	if fileErr != nil { // Check if path exists
		fmt.Println("Invalid Path `", dir, "`. Please specify a valid path.")
		os.Exit(1)
	}
	startServer() // start the file server
}

func startServer() {
	fmt.Printf("Starting %s with root %s on port %s.\nPress ctrl + c to exit.\n", strings.Title(NAME), dir, port)
	handler := HTTPLog(&fileServerHandler{http.Dir(dir)})
	http.Handle("/", handler)
	conErr := http.ListenAndServe("0.0.0.0:"+port, nil)
	if conErr != nil {
		fmt.Println(conErr)
		os.Exit(1)
	}
}

func getHomeDir() string {
	usr, err := user.Current()
	if err != nil {
		fmt.Println("Error getting user info.")
		os.Exit(1)
	}
	return usr.HomeDir
}

func handleExit() {
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt)
	for sig := range signalChannel {
		if sig == syscall.SIGINT {
			fmt.Printf("\n %s stopped.\n", NAME)
			os.Exit(0)
		}
	}
}

func urlEscape(s string) string {
	hexCount := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if shouldEscape(c) {
			hexCount++
		}
	}

	if hexCount == 0 {
		return s
	}

	t := make([]byte, len(s)+2*hexCount)
	j := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case shouldEscape(c):
			t[j] = '%'
			t[j+1] = "0123456789ABCDEF"[c>>4]
			t[j+2] = "0123456789ABCDEF"[c&15]
			j += 3
		default:
			t[j] = s[i]
			j++
		}
	}
	return string(t)
}

func shouldEscape(c byte) bool {
	// §2.3 Unreserved characters (alphanum)
	if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' {
		return false
	}

	switch c {
	case '-', '_', '.', '~': // §2.3 Unreserved characters (mark)
		return false

	case '$', '&', '+', ',', '/', ':', ';', '=', '?', '@': // §2.2 Reserved characters (reserved)
		// Different sections of the URL allow a few of
		// the reserved characters to appear unescaped.
		// The RFC reserves (so we must escape) everything.
		return true
	}

	// Everything else must be escaped.
	return true
}
