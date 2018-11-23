package quic

import (
	"io/ioutil"
	"bitbucket.com/marcmolla/gorl/agents"
	"bitbucket.com/marcmolla/gorl"
	"time"
	"bitbucket.com/marcmolla/gorl/types"
	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/utils"
	"errors"
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

func RewardFinalGoodput(duration time.Duration, _ time.Duration) types.Output {
	return types.Output(8) / types.Output(duration.Seconds()) * 5.
}

func GetStateAndReward(sch *scheduler, s *session) (types.Vector, types.Output, []*path){
	packetNumber := make(map[protocol.PathID]uint64)
	retransNumber := make(map[protocol.PathID]uint64)
	lossNumbers := make(map[protocol.PathID]uint64)

	sRTT := make(map[protocol.PathID]time.Duration)
	cwnd := make(map[protocol.PathID]protocol.ByteCount)
	cwndlevel := make(map[protocol.PathID]types.Output)

	firstPath, secondPath := protocol.PathID(255), protocol.PathID(255)

	for pathID, path := range s.paths{
		if pathID != protocol.InitialPathID{
			packetNumber[pathID], retransNumber[pathID], lossNumbers[pathID] = path.sentPacketHandler.GetStatistics()
			sRTT[pathID] = path.rttStats.SmoothedRTT()
			cwnd[pathID] = path.sentPacketHandler.GetCongestionWindow()
			cwndlevel[pathID] = types.Output(path.sentPacketHandler.GetBytesInFlight())/types.Output(cwnd[pathID])

			// Ordering paths
			if firstPath == protocol.PathID(255){
				firstPath = pathID
			}else{
				if pathID < firstPath{
					secondPath = firstPath
					firstPath = pathID
				}else{
					secondPath = pathID
				}
			}
		}
	}

	packetNumberInitial, _, _ := s.paths[protocol.InitialPathID].sentPacketHandler.GetStatistics()


	//Partial reward
	sentBytes := s.paths[firstPath].sentPacketHandler.GetSentBytes() + s.paths[secondPath].sentPacketHandler.GetSentBytes()
	partialReward := types.Output(sentBytes) * 8 / 1024/1024 / types.Output(time.Since(s.sessionCreationTime)) / 3500

	//Penalize and fast-quit
	if sch.Training{
		if packetNumberInitial > 20 || (retransNumber[firstPath]+retransNumber[secondPath]) > 0 || (lossNumbers[firstPath]+lossNumbers[secondPath]) > 0{
			utils.Errorf("closing: zero tolerance")
			sch.TrainingAgent.CloseEpisode(uint64(s.connectionID), -100, false)
			s.closeLocal(errors.New("closing: zero tolerance"))
		}
	}

	state := types.Vector{NormalizeTimes(sRTT[firstPath]), NormalizeTimes(sRTT[secondPath]),
	types.Output(cwnd[firstPath])/types.Output(protocol.DefaultTCPMSS)/300, types.Output(cwnd[secondPath])/types.Output(protocol.DefaultTCPMSS)/300,
	cwndlevel[firstPath], cwndlevel[secondPath],
	}

	return state, partialReward, []*path{s.paths[firstPath], s.paths[secondPath]}
}

func CheckAction(action int, state types.Vector, s *session, sch *scheduler){
	if action != 0{
		return
	}
	if state[4] < 1 || state[5] < 1 {
		// penalize not sending with one path allowed
		utils.Errorf("not sending with one path allowed")
		sch.TrainingAgent.CloseEpisode(uint64(s.connectionID), -100, false)
		s.closeLocal(errors.New("not sending with one path allowed"))
	}

}