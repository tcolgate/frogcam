package main // import "github.com/tcolgate/frogcam"

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	_ "net/http/pprof"
	"net/textproto"
	"os"
	"sort"
	"strconv"
	"strings"

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

type motionCam struct {
	cam     *webcam.Webcam
	timeout uint32
	active  map[io.Reader]struct{}
	fi      chan []byte
	li      chan *bytes.Buffer
	back    chan struct{}
	f       webcam.PixelFormat
	w, h    uint32
}

func newMotionCam(dev, fmtstr, szstr string) (*motionCam, error) {
	cam, err := webcam.Open(dev)
	if err != nil {
		return nil, err
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
		if fmtstr == "" {
			if supportedFormats[f] {
				format = f
				break FMT
			}

		} else if fmtstr == s {
			if !supportedFormats[f] {
				return nil, fmt.Errorf("format %q is not supporte", formatDesc[f])
			}
			format = f
			break
		}
	}
	if format == 0 {
		return nil, fmt.Errorf("No format found, exiting")
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
	case szstr == "":
		size = &frames[len(frames)-1]
	case strings.Count(szstr, "x") == 1:
		parts := strings.Split(szstr, "x")
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
			if szstr == f.GetString() {
				size = &f
			}
		}
	}

	if size == nil {
		return nil, fmt.Errorf("no matching frame size %q", szstr)
	}

	fmt.Fprintln(os.Stderr, "Requesting", formatDesc[format], size.GetString())
	f, w, h, err := cam.SetImageFormat(format, uint32(size.MaxWidth), uint32(size.MaxHeight))
	if err != nil {
		return nil, fmt.Errorf("SetImageFormat error %v", err)
	}
	fmt.Fprintf(os.Stderr, "Resulting image format: %s %dx%d\n", formatDesc[f], w, h)

	// start streaming
	err = cam.StartStreaming()
	if err != nil {
		return nil, fmt.Errorf("failed to start strea, %v", err)
	}

	return &motionCam{
		cam:     cam,
		timeout: uint32(15),
		fi:      make(chan []byte),
		back:    make(chan struct{}),
		li:      make(chan *bytes.Buffer),
		f:       f,
		w:       w,
		h:       h,
	}, nil

}

// motion jpeg frames are missing attributes for use as a
// regular jpeg. We add them back here.
func addMotionDht(frame []byte) []byte {
	var (
		dhtMarker = []byte{255, 196}
		dht       = []byte{1, 162, 0, 0, 1, 5, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 1, 0, 3, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 16, 0, 2, 1, 3, 3, 2, 4, 3, 5, 5, 4, 4, 0, 0, 1, 125, 1, 2, 3, 0, 4, 17, 5, 18, 33, 49, 65, 6, 19, 81, 97, 7, 34, 113, 20, 50, 129, 145, 161, 8, 35, 66, 177, 193, 21, 82, 209, 240, 36, 51, 98, 114, 130, 9, 10, 22, 23, 24, 25, 26, 37, 38, 39, 40, 41, 42, 52, 53, 54, 55, 56, 57, 58, 67, 68, 69, 70, 71, 72, 73, 74, 83, 84, 85, 86, 87, 88, 89, 90, 99, 100, 101, 102, 103, 104, 105, 106, 115, 116, 117, 118, 119, 120, 121, 122, 131, 132, 133, 134, 135, 136, 137, 138, 146, 147, 148, 149, 150, 151, 152, 153, 154, 162, 163, 164, 165, 166, 167, 168, 169, 170, 178, 179, 180, 181, 182, 183, 184, 185, 186, 194, 195, 196, 197, 198, 199, 200, 201, 202, 210, 211, 212, 213, 214, 215, 216, 217, 218, 225, 226, 227, 228, 229, 230, 231, 232, 233, 234, 241, 242, 243, 244, 245, 246, 247, 248, 249, 250, 17, 0, 2, 1, 2, 4, 4, 3, 4, 7, 5, 4, 4, 0, 1, 2, 119, 0, 1, 2, 3, 17, 4, 5, 33, 49, 6, 18, 65, 81, 7, 97, 113, 19, 34, 50, 129, 8, 20, 66, 145, 161, 177, 193, 9, 35, 51, 82, 240, 21, 98, 114, 209, 10, 22, 36, 52, 225, 37, 241, 23, 24, 25, 26, 38, 39, 40, 41, 42, 53, 54, 55, 56, 57, 58, 67, 68, 69, 70, 71, 72, 73, 74, 83, 84, 85, 86, 87, 88, 89, 90, 99, 100, 101, 102, 103, 104, 105, 106, 115, 116, 117, 118, 119, 120, 121, 122, 130, 131, 132, 133, 134, 135, 136, 137, 138, 146, 147, 148, 149, 150, 151, 152, 153, 154, 162, 163, 164, 165, 166, 167, 168, 169, 170, 178, 179, 180, 181, 182, 183, 184, 185, 186, 194, 195, 196, 197, 198, 199, 200, 201, 202, 210, 211, 212, 213, 214, 215, 216, 217, 218, 226, 227, 228, 229, 230, 231, 232, 233, 234, 242, 243, 244, 245, 246, 247, 248, 249, 250}
		sosMarker = []byte{255, 218}
	)
	jpegParts := bytes.Split(frame, sosMarker)
	return append(jpegParts[0], append(dhtMarker, append(dht, append(sosMarker, jpegParts[1]...)...)...)...)
}

func frameToImage(frame []byte, w, h uint32, format webcam.PixelFormat) (*image.YCbCr, []byte, error) {
	switch format {
	case fmtYUYV:
		img := image.NewYCbCr(image.Rect(0, 0, int(w), int(h)), image.YCbCrSubsampleRatio422)
		for i := range img.Cb {
			ii := i * 4
			img.Y[i*2] = frame[ii]
			img.Y[i*2+1] = frame[ii+2]
			img.Cb[i] = frame[ii+1]
			img.Cr[i] = frame[ii+3]

		}
		return img, frame, nil
	case fmtMJPEG:
		frame = addMotionDht(frame)
		bufr := bytes.NewReader(frame)
		img, err := jpeg.Decode(bufr)
		var ok bool
		yuv, ok := img.(*image.YCbCr)
		if !ok {
			return nil, nil, errors.New("not YUV image")
		}
		return yuv, frame, err
	default:
	}
	return nil, nil, errors.New("unknown format")
}

func encodeToJPEG(back chan struct{}, fi chan []byte, li chan *bytes.Buffer, w, h uint32, format webcam.PixelFormat) {
	var frame []byte
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
			var err error
			//convert to jpeg
			var img image.Image
			img, frame, err = frameToImage(frame, w, h, format)
			if err != nil {
				log.Fatal(err)
				return
			}
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

func httpVideo(li chan *bytes.Buffer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("connect from", r.RemoteAddr, r.URL)
		if r.URL.Path != "/stream" {
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
}

func (mc *motionCam) Run() error {
	go encodeToJPEG(mc.back, mc.fi, mc.li, mc.w, mc.h, mc.f)

	for {
		err := mc.cam.WaitForFrame(mc.timeout)
		switch err.(type) {
		case nil:
		case *webcam.Timeout:
			log.Println(err)
			continue
		default:
			return fmt.Errorf("unhandled error from WaitForFrame, %v", err)
		}

		frame, err := mc.cam.ReadFrame()
		if err != nil {
			return fmt.Errorf("unhandled error reading frame, %v", err)
		}
		if len(frame) != 0 {
			select {
			case mc.fi <- frame:
				<-mc.back
			default:
			}
		}
	}
}

func (mc *motionCam) GetImage() (image.Image, error) {
	return nil, nil
}

func (mc *motionCam) RecordMJPEF() (io.Reader, error) {
	return nil, nil
}

func (mc *motionCam) StopRecording(io.Reader) error {
	return nil
}
