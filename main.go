package main // import "github.com/tcolgate/frogcam"

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blackjack/webcam"
)

const (
	fmtYUYV  = 0x56595559
	fmtMJPEG = 0x47504a4d
)

type byArea []webcam.FrameSize

func (slice byArea) Len() int {
	return len(slice)
}

//For sorting purposes
func (slice byArea) Less(i, j int) bool {
	ls := slice[i].MaxWidth * slice[i].MaxHeight
	rs := slice[j].MaxWidth * slice[j].MaxHeight
	return ls < rs
}

//For sorting purposes
func (slice byArea) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

var supportedFormats = map[webcam.PixelFormat]bool{
	fmtYUYV:  true,
	fmtMJPEG: true,
}

func main() {
	dev := flag.String("d", "/dev/video0", "video device to use")
	fmtstr := flag.String("f", "", "video format to use, default first supported")
	szstr := flag.String("s", "", "frame size to use, default largest one")
	addr := flag.String("l", ":8080", "addr to listien")
	fps := flag.Bool("p", false, "print fps info")
	flag.Parse()

	cam, err := webcam.Open(*dev)
	if err != nil {
		panic(err.Error())
	}
	defer cam.Close()

	// select pixel format
	formatDesc := cam.GetSupportedFormats()

	fmt.Println("Available formats:")
	for f, s := range formatDesc {
		fmt.Fprintf(os.Stderr, "%s (%#x)\n", s, f)
	}

	var format webcam.PixelFormat
FMT:
	for f, s := range formatDesc {
		if *fmtstr == "" {
			if supportedFormats[f] {
				format = f
				break FMT
			}

		} else if *fmtstr == s {
			if !supportedFormats[f] {
				log.Println(formatDesc[f], "format is not supported, exiting")
				return
			}
			format = f
			break
		}
	}
	if format == 0 {
		log.Println("No format found, exiting")
		return
	}

	// select frame size
	frames := byArea(cam.GetSupportedFrameSizes(format))
	sort.Sort(frames)

	fmt.Fprintln(os.Stderr, "Supported frame sizes for format", formatDesc[format])
	for _, f := range frames {
		fmt.Fprintln(os.Stderr, f.GetString())
	}
	var size *webcam.FrameSize
	switch {
	case *szstr == "":
		size = &frames[len(frames)-1]
	case strings.Count(*szstr, "x") == 1:
		parts := strings.Split(*szstr, "x")
		x, xerr := strconv.Atoi(parts[0])
		y, yerr := strconv.Atoi(parts[1])
		if xerr != nil || yerr != nil {
			log.Fatalf("couldn't parse width x height")
		}
		size = &webcam.FrameSize{
			MaxWidth:  uint32(x),
			MaxHeight: uint32(y),
		}
	default:
		for _, f := range frames {
			if *szstr == f.GetString() {
				size = &f
			}
		}
	}

	if size == nil {
		log.Println("No matching frame size, exiting")
		return
	}

	fmt.Fprintln(os.Stderr, "Requesting", formatDesc[format], size.GetString())
	f, w, h, err := cam.SetImageFormat(format, uint32(size.MaxWidth), uint32(size.MaxHeight))
	if err != nil {
		log.Println("SetImageFormat return error", err)
		return

	}
	fmt.Fprintf(os.Stderr, "Resulting image format: %s %dx%d\n", formatDesc[f], w, h)

	// start streaming
	err = cam.StartStreaming()
	if err != nil {
		log.Println(err)
		return
	}

	var (
		li   chan *bytes.Buffer = make(chan *bytes.Buffer)
		fi                      = make(chan []byte)
		back                    = make(chan struct{})
	)

	go encodeToImage(cam, back, fi, li, w, h, f)
	go httpVideo(*addr, li)

	timeout := uint32(15) //5 seconds
	start := time.Now()
	var fr time.Duration

	for {
		err = cam.WaitForFrame(timeout)
		if err != nil {
			log.Println(err)
			return
		}

		switch err.(type) {
		case nil:
		case *webcam.Timeout:
			log.Println(err)
			continue
		default:
			log.Println(err)
			return
		}

		frame, err := cam.ReadFrame()
		if err != nil {
			log.Println(err)
			return
		}
		if len(frame) != 0 {

			// print framerate info every 10 seconds
			fr++
			if *fps {
				if d := time.Since(start); d > time.Second*10 {
					fmt.Println(float64(fr)/(float64(d)/float64(time.Second)), "fps")
					start = time.Now()
					fr = 0
				}
			}

			select {
			case fi <- frame:
				<-back
			default:
			}
		}
	}
}

func encodeToImage(wc *webcam.Webcam, back chan struct{}, fi chan []byte, li chan *bytes.Buffer, w, h uint32, format webcam.PixelFormat) {

	var (
		frame []byte
	)
	for {
		bframe := <-fi
		// copy frame
		if len(frame) < len(bframe) {
			frame = make([]byte, len(bframe))
		}
		copy(frame, bframe)
		back <- struct{}{}
		buf := &bytes.Buffer{}

		switch format {
		case fmtYUYV:
			var img image.Image
			yuyv := image.NewYCbCr(image.Rect(0, 0, int(w), int(h)), image.YCbCrSubsampleRatio422)
			for i := range yuyv.Cb {
				ii := i * 4
				yuyv.Y[i*2] = frame[ii]
				yuyv.Y[i*2+1] = frame[ii+2]
				yuyv.Cb[i] = frame[ii+1]
				yuyv.Cr[i] = frame[ii+3]

			}
			img = yuyv
			//convert to jpeg
			if err := jpeg.Encode(buf, img, nil); err != nil {
				log.Fatal(err)
				return
			}
		case fmtMJPEG:
			buf = bytes.NewBuffer(frame)
		default:
			log.Fatal("invalid format ?")
		}

		const N = 50
		// broadcast image up to N ready clients
		nn := 0
	FOR:
		for ; nn < N; nn++ {
			select {
			case li <- buf:
			default:
				break FOR
			}
		}
		if nn == 0 {
			li <- buf
		}
	}
}

func httpVideo(addr string, li chan *bytes.Buffer) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println("connect from", r.RemoteAddr, r.URL)
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		//remove stale image
		<-li
		multipartWriter := multipart.NewWriter(w)
		var boundary = multipartWriter.Boundary()
		log.Println("boundary = ", boundary)
		w.Header().Set("Content-Type", `multipart/x-mixed-replace;boundary=`+boundary)
		//multipartWriter.SetBoundary(boundary)
		for {
			img := <-li
			image := img.Bytes()
			iw, err := multipartWriter.CreatePart(textproto.MIMEHeader{
				"Content-type":   []string{"image/jpeg"},
				"Content-length": []string{strconv.Itoa(len(image))},
			})
			if err != nil {
				log.Println(err)
				return
			}
			_, err = iw.Write(image)
			if err != nil {
				log.Println(err)
				return
			}
		}
	})

	log.Fatal(http.ListenAndServe(addr, nil))
}
