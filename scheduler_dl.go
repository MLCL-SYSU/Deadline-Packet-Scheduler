package quic

import (
	"io/ioutil"
	"bitbucket.com/marcmolla/gorl/agents"
	"bitbucket.com/marcmolla/gorl"
	"time"
	"bitbucket.com/marcmolla/gorl/types"
	"github.com/lucas-clemente/quic-go/internal/protocol"
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
	if specFile != "" {
		spec, err = ioutil.ReadFile(specFile)
		if err != nil {
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

/*func NormalizeQuotas(quota1, quota2 uint) [2]types.Output{
	q1 := types.Output(quota1)
	q2 := types.Output(quota2)

	if quota1 > quota2{
		return [2]types.Output{1., q2/q1}
	}else if quota1 < quota2{
		return [2]types.Output{q1/q2, 1.}
	}
	return [2]types.Output{0., 0.}
}*/

func RewardFinalGoodput(duration time.Duration, _ time.Duration) types.Output {
	return types.Output(8) / types.Output(duration.Seconds()) * 5.
}

//TODO: Maximize the windows size?? vs allowing negative quota
func RewardPartial(ackdBytes protocol.ByteCount, elapsed time.Duration, retrans bool) types.Output{
	//return (types.Output(ackdBytes) * 8 / 1024/1024 / types.Output(elapsed.Seconds())) / 50
	mul := 1
	if retrans{
		mul = -1
	}
	return types.Output(mul)* types.Output(ackdBytes) * 8 / 1024/1024 / types.Output(elapsed.Seconds())
}