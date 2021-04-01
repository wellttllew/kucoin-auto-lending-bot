package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/Kucoin/kucoin-go-sdk"
	"github.com/sirupsen/logrus"
)

func main() {

	// override kucoin-go-sdk package "init" side effect
	logrus.SetOutput(os.Stdout)

	config, err := LoadConfigFromEnv()

	if err != nil {
		logrus.Fatalf("failed to load config: %v", err)
	}

	cli := kucoin.NewApiService(
		// kucoin.ApiBaseURIOption("https://api.kucoin.com"),
		kucoin.ApiKeyOption(config.APIKey),
		kucoin.ApiSecretOption(config.APISecret),
		kucoin.ApiPassPhraseOption(config.APIPassPhrase),
		kucoin.ApiKeyVersionOption(kucoin.ApiKeyVersionV2),
	)

	type Step int
	const (
		// Step1 : check available USDT
		StepCheckAvailableUSDT Step = iota
		// Step2 : get minimum dayly interest rate
		StepGetMinDayIntRate
		// Step3 : create order
		StepCreateOrder
		// Step4 : wait for order to be filled
		StepWaitOrderFill
		// Step5 : cancel unfilled order
		StepCancelOrder
	)

	nextStep := StepCheckAvailableUSDT

	currentAvailableUSDT := float64(0)
	orderID := ""
	rate := float64(0)

LENDING_LOOP:
	for {
		switch nextStep {

		case StepCheckAvailableUSDT:
			currentAvailableUSDT, err = GetCurrentAvailableUSDT(cli, config.ReservedAmount)
			if err != nil {
				logrus.Warnf("failed to get available USDT: %v ", err)
				time.Sleep(time.Second * 1)
				continue LENDING_LOOP
			}
			if currentAvailableUSDT < minimumOrderAmount {
				logrus.Warnf("no enough amount available : %v ", currentAvailableUSDT)
				time.Sleep(time.Minute * 5)
				continue LENDING_LOOP
			}
			nextStep = StepGetMinDayIntRate

		case StepGetMinDayIntRate:
			rate, err = GetMinDayIntRate(cli)
			if err != nil {
				logrus.Warnf("failed to get current minimum daily interest rate: %v", rate)
				continue LENDING_LOOP
			}

			if rate < config.MinDailyIntRate {
				logrus.Warnf("%v is less than expected minimum rate %v, using %v", rate, config.MinDailyIntRate, config.MinDailyIntRate)
				rate = config.MinDailyIntRate
			}

			nextStep = StepCreateOrder

		case StepCreateOrder:
			orderID, err = CreateLendOrder(cli, currentAvailableUSDT, rate)
			if err != nil {
				logrus.Warnf("failed to create order: %v", err)
				nextStep = StepGetMinDayIntRate
				continue LENDING_LOOP
			}

			logrus.Infof("order(id=%v) created, amount: %v, rate: %v", orderID, currentAvailableUSDT, rate)

			nextStep = StepWaitOrderFill

		case StepWaitOrderFill:
			ticker := time.NewTicker(time.Second * 10)
			timer := time.NewTimer(time.Minute * 5)
			for {
				select {
				case <-ticker.C:
					status, err := CheckLendOrder(cli, orderID)
					if err != nil {
						logrus.Warnf("failed to check lend order status: %v", err)
						break // select
					}
					logrus.Infof("order (id=%v) status: %v", orderID, status)

					if status == FULLY_FILLED_ORDER {
						logrus.Infof("order (id=%v) filled", orderID)
						nextStep = StepCheckAvailableUSDT
						timer.Stop()
						continue LENDING_LOOP
					}
				case <-timer.C:
					logrus.Warnf("timeout when waiting for orders to be filled")
					nextStep = StepCancelOrder
					ticker.Stop()
					continue LENDING_LOOP
				}
			}

		case StepCancelOrder:

			// continue trying to cancel the order until succeeded.
			for {
				if err := CancelOrder(cli, orderID); err == nil {
					logrus.Infof("order (id=%v) filled or cancelled", orderID)
					nextStep = StepCheckAvailableUSDT
					continue LENDING_LOOP
				}
				time.Sleep(time.Second * 10)
			}

		} // end of select
	} // end of for loop

} // end of main

// get current available USDT
func GetCurrentAvailableUSDT(cli *kucoin.ApiService, reserved float64) (amount float64, err error) {
	res, err := cli.Accounts("USDT", "main")
	if err != nil {
		return 0, fmt.Errorf("failed to list accounts: %v", err.Error())
	}

	var accounts kucoin.AccountsModel

	if err = res.ReadData(&accounts); err != nil {
		return 0, fmt.Errorf("failed to read accounts: %v", err.Error())
	}

	if len(accounts) < 1 {
		return 0, fmt.Errorf("got no main account")
	}

	totalAvailable, err := strconv.ParseFloat(accounts[0].Available, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %v as float64: %v", accounts[0].Available, err.Error())
	}

	amount = totalAvailable - reserved

	if amount < 0 {
		amount = 0
	}

	return math.Floor(amount), nil

}

// get current minimum daily interest rate
func GetMinDayIntRate(cli *kucoin.ApiService) (rate float64, err error) {

	var orderLst kucoin.MarginMarketsModel

	r, err := cli.MarginMarkets(map[string]string{
		"currency": "USDT",
		"term":     "7",
	})

	if err != nil {
		return 0, fmt.Errorf("failed to list lending orders: %v", err)
	}

	if err = r.ReadData(&orderLst); err != nil {
		return 0, fmt.Errorf("failed to read lending order list : %v", err)
	}

	if len(orderLst) < 1 {
		return 0, fmt.Errorf("got a zero length order list")
	}

	minRate, err := orderLst[0].DailyIntRate.Float64()

	if minRate < epsilon {
		return 0, fmt.Errorf("a rate of %v is too small", minRate)
	}

	return minRate, nil
}

func CreateLendOrder(cli *kucoin.ApiService, amount float64, rate float64) (orderID string, err error) {

	r, err := cli.CreateLendOrder(map[string]string{
		"currency":     "USDT",
		"size":         fmt.Sprint(amount),
		"dailyIntRate": fmt.Sprint(rate),
		"term":         "7",
	})

	if err != nil {
		return "", fmt.Errorf("failed to create lending order: %v", err)
	}

	var order kucoin.CreateLendOrderResultModel

	if err = r.ReadData(&order); err != nil {
		return "", fmt.Errorf("failed to read order id : %v", err)
	}

	if order.OrderId == "" {
		return "", fmt.Errorf("got an empty order id")
	}

	return order.OrderId, nil

}

type orderFillStatus string

const (
	FULLY_FILLED_ORDER     orderFillStatus = "fully filled"
	NOT_FULLY_FILLED_ORDER orderFillStatus = "not fully filled"
	UNKNOWN                orderFillStatus = "unknown"
)

func CheckLendOrder(cli *kucoin.ApiService, orderID string) (orderFillStatus, error) {

	r, err := cli.LendActiveOrders("USDT", &kucoin.PaginationParam{
		CurrentPage: 1,
		PageSize:    50,
	})

	if err != nil {
		return UNKNOWN, fmt.Errorf("failed to get lending order: %v", err)
	}

	var singlePageResult kucoin.PaginationModel

	if err = r.ReadData(&singlePageResult); err != nil {
		return UNKNOWN, fmt.Errorf("failed to read active lending orders: %v", err)
	}

	if singlePageResult.TotalPage > 1 {
		return UNKNOWN, fmt.Errorf("Too many active lending orders")
	}

	var orders kucoin.LendActiveOrdersModel
	if err = singlePageResult.ReadItems(&orders); err != nil {
		return UNKNOWN, fmt.Errorf("failed to read orders from page result: %v", err)
	}

	for _, o := range orders {
		if o.OrderId == orderID {
			return NOT_FULLY_FILLED_ORDER, nil
		}
	}

	return FULLY_FILLED_ORDER, nil
}

func CancelOrder(cli *kucoin.ApiService, orderID string) error {
	r, err := cli.CancelLendOrder(orderID)

	if err != nil {
		return fmt.Errorf("failed to cancel lending order: %v", err)
	}

	const (
		ALREADY_FILLED  = "210010"
		ORDER_NOT_FOUND = "210005"
		SUCCESS         = "200000"
	)

	if r.Code == ALREADY_FILLED || r.Code == SUCCESS {
		return nil
	}

	return errors.New(r.Message)
}

// a float number that is small enough
const epsilon float64 = 0.000000001
const minimumOrderAmount float64 = 10

// configs
type ConfigType struct {
	MinDailyIntRate float64 `json:"minDailyIntRate"`
	APIKey          string  `json:"apiKey"`
	APISecret       string  `json:"apiSecret"`
	APIPassPhrase   string  `json:"apiPassPhrase"`
	ReservedAmount  float64 `json:"reservedAmount"`
}

// Load config from environment variable
func LoadConfigFromEnv() (*ConfigType, error) {

	var c ConfigType
	var err error

	c.MinDailyIntRate, err = strconv.ParseFloat(os.Getenv("MIN_DAILY_INT_RATE"), 64)
	if err != nil {
		return nil, fmt.Errorf("failed to read 'MIN_DAILY_INT_RATE' from environment variable: %v", err)
	}

	c.ReservedAmount, err = strconv.ParseFloat(os.Getenv("RESERVED_USDT_AMOUNT"), 64)
	if err != nil {
		return nil, fmt.Errorf("failed to read 'RESERVED_USDT_AMOUNT' from environment variable: %v", err)
	}

	c.APIKey = os.Getenv("KUCOIN_API_KEY")
	if c.APIKey == "" {
		return nil, fmt.Errorf("missing 'KUCOIN_API_KEY' in environment variable")
	}

	c.APISecret = os.Getenv("KUCOIN_API_SECRET")
	if c.APISecret == "" {
		return nil, fmt.Errorf("missing 'KUCOIN_API_SECRET' in environment variable")
	}

	c.APIPassPhrase = os.Getenv("KUCOIN_API_PASSPHRASE")
	if c.APIPassPhrase == "" {
		return nil, fmt.Errorf("missing 'KUCOIN_API_PASSPHRASE' in environment variable")
	}

	return &c, nil
}
