package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Draal/gorf24"
)

const RPI_V2_GPIO_P1_15 = 22       ///< Version 2, Pin P1-15
const RPI_V2_GPIO_P1_24 = 8        ///< Version 2, Pin P1-24, CE0 when SPI0 in use
const BCM2835_SPI_SPEED_1MHZ = 256 //BCM2835_SPI_CLOCK_DIVIDER_256

var addresses = []uint64{0xABCDABCD71, 0x544d52687C}

var data = []byte("Draal is the best")

type Cmd uint8

const (
	CmdPong Cmd = 2
	CmdTemp Cmd = 5
)

type Msg struct {
	Cmd  Cmd
	Val1 int32
	Val2 int32
}

func readTemp(ctx context.Context, radio *gorf24.R) (resp Msg, err error) {
	tm := time.NewTicker(50 * time.Millisecond)
	defer tm.Stop()
	radio.StartListening()
	for {
		if radio.Available() {
			break
		}
		select {
		case <-tm.C:
		case <-ctx.Done():
			return resp, fmt.Errorf("reading context timeout %s", ctx.Err())
		}
	}
	val := radio.Read(9)
	if err := binary.Read(bytes.NewReader((val)), binary.LittleEndian, &resp); err != nil {
		return resp, fmt.Errorf("Failed to read structure %v: %s", val, err.Error())
	}
	return
}

func planForProduct(product string) (time.Time, func(timeIn time.Duration) int32) {
	switch product {
	case "brisket":
		return time.Now().Add(6 * time.Hour), func(timeIn time.Duration) int32 {
			return 165
		}
	default:
		panic(fmt.Sprintf("Unknown product %s", product))
	}
}
func main() {
	var (
		output  = flag.String("output", "", "csv output path")
		product = flag.String("product", "brisket", "smoking type. eg. brisket,ribs")
	)
	flag.Parse()
	if *output == "" {
		flag.Usage()
		log.Fatal("Output path is required")
	}

	fp, err := os.OpenFile(*output, os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("Failed to open path %w", err)
	}
	defer fp.Close()
	fmt.Fprintf(fp, "fromStart,time,tempF\n")

	radio := gorf24.New(RPI_V2_GPIO_P1_15, RPI_V2_GPIO_P1_24, BCM2835_SPI_SPEED_1MHZ)
	radio.Begin()
	radio.SetChannel(0x76)
	radio.SetPALevel(gorf24.PA_MAX)
	radio.SetDataRate(gorf24.RATE_1MBPS)
	radio.SetPayloadSize(9)
	radio.SetAutoAck(true)
	radio.SetRetries(2, 15)
	radio.SetCRCLength(gorf24.CRC_8BIT)
	radio.PrintDetails()

	radio.OpenWritingPipe(addresses[1])
	radio.OpenReadingPipe(1, addresses[0])

	start := time.Now()
	type tempMark struct {
		FromStart string
		Temp      float64
	}
	var (
		fromStartStamp      string
		fromStart           time.Duration
		lastTemps           = make([]tempMark, 1, 5)
		curValueIndex       = 0
		lastUpdated         time.Time
		wrapped             bool
		wrappedAt           time.Time
		wrappAt, tempPlanFn = planForProduct(*product)
	)
	http.HandleFunc("/tempz", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("wrapped") != "" {
			wrapped = true
			wrappedAt = time.Now()
		}
		fmt.Fprintf(w, `<html>
			<head>
				<meta http-equiv="refresh" content="25">
			</head>
			<body><h1>From start <b>%s</b><br/>Current temp <b style="color:red">%2.2f</b> (Target <b>%d</b>)</h1>`,
			lastTemps[curValueIndex].FromStart, lastTemps[curValueIndex].Temp, tempPlanFn(fromStart))
		if d := time.Since(lastUpdated); d > 1*time.Minute {
			fmt.Fprintf(w, `<h2 style="color:red">STALE DATA!!!! last updated %s</h2>`, d)
		}
		if wrapped {
			fmt.Fprintf(w, `<h2 style="color:green">wrapped at %s</h2>`, wrappedAt.Format(time.RFC1123))
		} else {
			fmt.Fprintf(w, `<h2 style="color:blue">wrapp at %s (in %.2fm)</h2>`, wrappAt.Format(time.Kitchen), wrappAt.Sub(time.Now()).Minutes())
		}
		fmt.Fprintf(w, `</body></html>`)
	})
	go func() {
		log.Fatal(http.ListenAndServe(":8081", nil))
	}()

	for {
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			resp, err := readTemp(ctx, &radio)
			fromStart = time.Since(start)
			fromStartMins := fromStart.Minutes()
			hours := int(fromStartMins / 60)
			mins := int(fromStartMins) - hours*60
			fromStartStamp = fmt.Sprintf("%02d:%02d", hours, mins)
			if err != nil {
				log.Printf("%s: Failed to read temp: %s\n", fromStartStamp, err.Error())
				return
			}
			if resp.Cmd != CmdTemp {
				log.Printf("Receive not temp response %d\n", resp.Cmd)
				return
			}
			temp := float64(resp.Val1) / 100
			mark := tempMark{
				FromStart: fromStartStamp,
				Temp:      temp,
			}
			if len(lastTemps) < 5 {
				curValueIndex = len(lastTemps)
				lastTemps = append(lastTemps, mark)
			} else {
				if curValueIndex == len(lastTemps)-1 {
					curValueIndex = 0
				}
				lastTemps[curValueIndex] = mark
				curValueIndex++
			}
			lastUpdated = time.Now()
			fmt.Fprintf(fp, "%s,%s,%f,%v\n", fromStartStamp, time.Now().Format(time.RFC1123), temp, wrapped)
			log.Printf("%s: Current temp %f\n", fromStartStamp, temp)
		}()
	}
}
