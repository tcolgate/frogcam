package main // import "github.com/tcolgate/frogcam"

import (
	"image"
	"log"
	_ "net/http/pprof"
	"time"

	"github.com/disintegration/gift"
	"github.com/harrydb/go/img/grayscale"
)

type motion []grayscale.CoCo

type motionDetector struct {
	t   *time.Ticker
	cam *motionCam
}

func newMotionDetector(mc *motionCam, t *time.Ticker) *motionDetector {
	return &motionDetector{
		t,
		mc,
	}
}

func (md *motionDetector) Run() {
	for {
		<-md.t.C
		img, err := md.cam.GetImage()
		if err != nil {
			log.Printf("err: %v", err)
			continue
		}
		sd.Update(img)

		/*
			dstImage := imaging.Blur(sd.e, 5)
		*/
		g := gift.New(
			gift.GaussianBlur(5),
		)
		dst := image.NewGray(g.Bounds(sd.e.Bounds()))
		g.Draw(dst, sd.e)

		cocos := grayscale.CoCos(dst, 255, grayscale.NEIGHBOR8)
		filteredCoCos := cocos[:0]

		for i := range cocos {
			if len(cocos[i]) > minCoCo {
				filteredCoCos = append(filteredCoCos, cocos[i])
			}
		}

		if len(filteredCoCos) > 0 {
			ms <- motion(filteredCoCos)
		}

		/*
			pal := colorful.FastWarmPalette(len(filteredCoCos))
			for i := range filteredCoCos {
				log.Printf("filteredCoCos[%d]: %d points, %v", i, len(filteredCoCos[i]), pal[i])
			}
		*/
	}
}
