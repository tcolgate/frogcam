package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"sync"
)

type sigmadelta struct {
	sync.RWMutex
	n      int
	bounds image.Rectangle

	m *image.Gray
	o *image.Gray
	v *image.Gray
	e *image.Gray
}

func newSigmaDelta(n int, bounds image.Rectangle) *sigmadelta {
	return &sigmadelta{
		n:      n,
		bounds: bounds,

		m: image.NewGray(bounds),
		o: image.NewGray(bounds),
		v: image.NewGray(bounds),
		e: image.NewGray(bounds),
	}
}

func (s *sigmadelta) Update(in *image.YCbCr) error {
	s.Lock()
	defer s.Unlock()

	// mt estimator
	for i := s.bounds.Min.X; i <= s.bounds.Max.X; i++ {
		for j := s.bounds.Min.Y; j <= s.bounds.Max.Y; j++ {
			inx := in.YCbCrAt(i, j).Y
			mx := s.m.GrayAt(i, j)
			switch {
			case mx.Y < inx:
				s.m.SetGray(i, j, color.Gray{mx.Y + 1})
			case mx.Y > inx:
				s.m.SetGray(i, j, color.Gray{mx.Y - 1})
			default:
			}
		}
	}

	// ot computation
	for i := s.bounds.Min.X; i <= s.bounds.Max.X; i++ {
		for j := s.bounds.Min.Y; j <= s.bounds.Max.Y; j++ {
			inx := in.YCbCrAt(i, j).Y
			mx := s.m.GrayAt(i, j)

			oy := int(mx.Y) - int(inx)
			if oy <= 0 {
				oy *= -1
			}

			s.o.SetGray(i, j, color.Gray{uint8(oy)})
		}
	}

	// vt update
	for i := s.bounds.Min.X; i <= s.bounds.Max.X; i++ {
		for j := s.bounds.Min.Y; j <= s.bounds.Max.Y; j++ {
			ox := s.o.GrayAt(i, j)
			vx := s.v.GrayAt(i, j)
			intvx := int(vx.Y)

			not := s.n * int(ox.Y)
			switch {
			case intvx < not:
				if intvx < 255 {
					s.v.SetGray(i, j, color.Gray{vx.Y + 1})
				}
			case intvx > not:
				if intvx > 1 {
					s.v.SetGray(i, j, color.Gray{vx.Y - 1})
				}
			default:
			}
		}
	}

	// et estimate
	for i := s.bounds.Min.X; i <= s.bounds.Max.X; i++ {
		for j := s.bounds.Min.Y; j <= s.bounds.Max.Y; j++ {
			ox := s.o.GrayAt(i, j)
			vx := s.v.GrayAt(i, j)

			switch {
			case ox.Y < vx.Y:
				s.e.SetGray(i, j, color.Gray{0})
			default:
				s.e.SetGray(i, j, color.Gray{255})
			}
		}
	}

	return nil
}

func (s *sigmadelta) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.RLock()
	defer s.RUnlock()

	switch r.URL.Path {
	case "":
		fmt.Fprintf(w, "<html><body>Hello</body></html>")
	case "m":
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, s.m)
	case "o":
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, s.o)
	case "v":
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, s.v)
	case "e":
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, s.e)
	default:
		http.Error(w, "file not found", 404)
	}
}
