package quic

import (
	"io/ioutil"
	"bitbucket.com/marcmolla/gorl/agents"
	"bitbucket.com/marcmolla/gorl"
	"time"
	"bitbucket.com/marcmolla/gorl/types"
)

var(
	maxGoodput = map[int]float64{
		0:		0.898005552,
		50:		0.976690572,
		100:	1.150461995,
		150:	1.293399122,
		200:	1.38544436,
		250:	1.445745136,
		300:	1.485911182,
		700:	1.649954398,
	}
)

func GetAgent(weightsFile string, specFile string) agents.Agent{
	var spec []byte
	var err error
	if specFile != ""{
		spec, err = ioutil.ReadFile(specFile)
		if err != nil{
			panic(err)
		}
	}
	agent := gorl.GetNormalInstance(string(spec))
	if weightsFile != ""{
		err = agent.LoadWeights(weightsFile)
		if err != nil{
			panic(err)
		}
	}
	return agent
}

func GetTrainingAgent(weightsFile string, specFile string, outputPath string, epsilon float64) agents.TrainingAgent{
	var spec []byte
	var err error
	if specFile != ""{
		spec, err = ioutil.ReadFile(specFile)
		if err != nil{
			panic(err)
		}
	}

	agent := gorl.GetTrainingInstance(string(spec), outputPath, float32(epsilon))
	if weightsFile != ""{
		err = agent.LoadWeights(weightsFile)
		if err != nil{
			panic(err)
		}
	}
	return agent
}

func NormalizeTimes(stat time.Duration) types.Output{
	return types.Output(stat.Nanoseconds()) / types.Output(time.Millisecond.Nanoseconds()*150)
}

func RewardFinalGoodput(duration time.Duration, maxRTT time.Duration) types.Output {
	mGoodput := maxGoodput[getTestRTT(maxRTT)]
	return types.Output(mGoodput/duration.Seconds() * 10000)
}

func RewardPartial(goodput float64) types.Output{
	return types.Output(goodput / 10)
}

func getTestRTT(rtt time.Duration)int{
	switch{
	case rtt > 700:
		return 700
	case rtt > 300:
		return 300
	case rtt > 250:
		return 250
	case rtt > 200:
		return 200
	case rtt > 150:
		return 150
	case rtt > 100:
		return 100
	case rtt > 50:
		return 50
	default:
		return 0
	}
}