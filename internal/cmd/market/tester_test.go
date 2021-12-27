package market

import (
	"io/ioutil"
	"testing"
	"time"

	"follow.market/internal/pkg/strategy"
	"follow.market/pkg/config"
	"github.com/sdcoffey/big"
	"github.com/stretchr/testify/assert"
)

func Test_Tester(t *testing.T) {
	consigPath := "./../../../configs/deploy.configs.json"
	configs, err := config.NewConfigs(&consigPath)
	assert.EqualValues(t, nil, err)
	assert.EqualValues(t, "development", configs.Stage)

	ticker := "BTCUSDT"
	signalPath := "./../../../configs/signals/signal.json"
	raw, err := ioutil.ReadFile(signalPath)
	assert.EqualValues(t, nil, err)

	signal, err := strategy.NewSignalFromBytes(raw)
	assert.EqualValues(t, nil, err)

	tester, err := newTester(initSharedParticipants(configs))
	assert.EqualValues(t, nil, err)

	stg := strategy.Strategy{
		EntryRule:      strategy.NewRule(*signal),
		ExitRule:       nil,
		RiskRewardRule: strategy.NewRiskRewardRule(-0.02, 0.04),
	}

	rs, err := tester.test(ticker, big.NewDecimal(10000), &stg, time.Now().AddDate(0, -1, 0), time.Now())
	assert.EqualValues(t, nil, err)
	assert.EqualValues(t, len(rs.record.Trades), len(rs.record.Trades))
}
