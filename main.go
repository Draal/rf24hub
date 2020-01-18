package main

import "github.com/Draal/gorf24"

const RPI_V2_GPIO_P1_15 = 22       ///< Version 2, Pin P1-15
const RPI_V2_GPIO_P1_24 = 8        ///< Version 2, Pin P1-24, CE0 when SPI0 in use
const BCM2835_SPI_SPEED_1MHZ = 256 //BCM2835_SPI_CLOCK_DIVIDER_256

func main() {
	radio := gorf24.New(RPI_V2_GPIO_P1_15, RPI_V2_GPIO_P1_24, BCM2835_SPI_SPEED_1MHZ)
	radio.Begin()
	radio.SetChannel(0x76)
	radio.SetPALevel(gorf24.PA_MAX)
	radio.SetDataRate(gorf24.RATE_1MBPS)
	radio.SetPayloadSize(32)
	radio.SetAutoAck(true)
	radio.SetRetries(2, 15)
	radio.SetCRCLength(gorf24.CRC_8BIT)
	radio.PrintDetails()
}
