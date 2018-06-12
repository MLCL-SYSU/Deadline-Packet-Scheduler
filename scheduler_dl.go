package quic

import (
	"io/ioutil"
	"bitbucket.com/marcmolla/gorl/agents"
	"bitbucket.com/marcmolla/gorl"
	"time"
	"bitbucket.com/marcmolla/gorl/types"
	"github.com/lucas-clemente/quic-go/internal/protocol"
)

var(
	maxGoodput = map[int]float64{
		0:		0.100312373,
		10:		0.113738699,
		20:		0.12774865,
		30:		0.137545458,
		40:		0.143708207,
		50:		0.149168649,
		60:		0.155744972,
		70:		0.159616985,
	}
	testingRTT = 0
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

func GetTrainingAgent(weightsFile string, specFile string, outputPath string, epsilon float64, rtt int) agents.TrainingAgent{
	var spec []byte
	var err error
	if specFile != ""{
		spec, err = ioutil.ReadFile(specFile)
		if err != nil{
			panic(err)
		}
	}
	if rtt >= 0{
		testingRTT = rtt
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

func NormalizeQuotas(quota1, quota2 uint) [2]types.Output{
	q1 := types.Output(quota1)
	q2 := types.Output(quota2)

	if quota1 > quota2{
		return [2]types.Output{1., q2/q1}
	}else if quota1 < quota2{
		return [2]types.Output{q1/q2, 1.}
	}
	return [2]types.Output{0., 0.}
}

func RewardFinalGoodput(duration time.Duration, _ time.Duration) types.Output {
	mGoodput := maxGoodput[testingRTT]
	return types.Output(mGoodput/duration.Seconds())
}

func RewardPartial(ackdBytes protocol.ByteCount, elapsed time.Duration) types.Output{
	//return (types.Output(ackdBytes) * 8 / 1024/1024 / types.Output(elapsed.Seconds())) / 50
	return types.Output(ackdBytes) * 8 / 1024/1024 / types.Output(elapsed.Seconds())
}