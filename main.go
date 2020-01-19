package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
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
	CmdPing         Cmd = 1
	CmdPong         Cmd = 2
	CmdReadMoisture Cmd = 3
	CmdDelay        Cmd = 4
)

type Msg struct {
	Cmd  Cmd
	Val1 int32
	Val2 int32
}

func sendCMD(ctx context.Context, m Msg, radio *gorf24.R) (resp Msg, err error) {
	var buf bytes.Buffer
	buf.Grow(9)
	if err = binary.Write(&buf, binary.LittleEndian, m); err != nil {
		return resp, fmt.Errorf("Failed to write structure: %s", err.Error())
	}
	radio.StopListening()
	log.Printf("Sending %v (%d)\n", m, buf.Len())
	tm := time.NewTicker(50 * time.Millisecond)
	defer tm.Stop()
	for {
		if radio.Write(buf.Bytes(), uint8(buf.Len())) {
			break
		}
		select {
		case <-tm.C:
		case <-ctx.Done():
			return resp, fmt.Errorf("Writing context timeout %s", ctx.Err())
		}
	}
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

func main() {
	radio := gorf24.New(RPI_V2_GPIO_P1_15, RPI_V2_GPIO_P1_24, BCM2835_SPI_SPEED_1MHZ)
	radio.Begin()
	radio.SetChannel(0x76)
	radio.SetPALevel(gorf24.PA_HIGH)
	radio.SetDataRate(gorf24.RATE_1MBPS)
	radio.SetPayloadSize(9)
	radio.SetAutoAck(true)
	radio.SetRetries(2, 15)
	radio.SetCRCLength(gorf24.CRC_8BIT)
	radio.PrintDetails()

	radio.OpenWritingPipe(addresses[1])
	radio.OpenReadingPipe(1, addresses[0])

	for {
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			m := Msg{
				Cmd:  CmdPing,
				Val1: rand.Int31(),
			}
			resp, err := sendCMD(ctx, m, &radio)
			if err != nil {
				log.Printf("Failed to send ping %v: %s\n", m, err.Error())
				return
			}
			if resp.Cmd != CmdPong {
				log.Printf("Receive not pong resonve on ping %d\n", resp.Cmd)
				return
			}
			if resp.Val1 != m.Val1 {
				log.Printf("Expected %d val1 but got %d\n", m.Val1, resp.Val1)
				return
			}
			log.Printf("Ping %v pong, %v\n", m, resp)
			moisture, err := sendCMD(ctx, Msg{
				Cmd: CmdReadMoisture,
			}, &radio)
			if err != nil {
				log.Printf("Failed to send read moisture %v: %s\n", m, err.Error())
				return
			}
			log.Printf("Current moisture %d\n", moisture.Val1)
			_, err = sendCMD(ctx, Msg{
				Cmd:  CmdDelay,
				Val1: 10000,
			}, &radio)
			if err != nil {
				log.Printf("Failed to delay %v: %s\n", m, err.Error())
				return
			}
		}()
		time.Sleep(500 * time.Millisecond)
	}
}
