package quic

import (
	"fmt"
	"github.com/draffensperger/golp"
	"github.com/lucas-clemente/quic-go/internal/protocol"
	"math/rand"
	"time"
)

// some parameter
//const alpha = 1.15 //alpha must large than 1
const banditAvaiable = true

func linOpt(packetsNum []int, packetsDeadline []float64, pathDelay []float64, pathCwnd []float64) []int {
	// TODO:packetsNum is unnecessary
	S := len(packetsNum)     // num of packets
	n := len(pathDelay)      // num of path
	policy := make([]int, S) // scheduler decision: [1 2] means packet 1 schedule in path 1, packet 2 schedule in path 2

	// Constraint coefficient matrix
	A := make([][]float64, S+n)
	for i := range A {
		A[i] = make([]float64, S*n)
	}
	// packet constraint: decision of one packet in all path leq 1
	for i := 0; i < S; i++ {
		for j := i; j < S*n; j += S {
			A[i][j] = 1
		}
	}
	// CWND constraint: packets in one path leq CWND
	for i := 0; i < n; i++ {
		for j := (i * S); j < ((i + 1) * S); j++ {
			A[S+i][j] = 1
		}
	}

	b := make([]float64, S+n)
	for i := 0; i < S; i++ {
		b[i] = 1
	}
	copy(b[S:], pathCwnd)

	// Objective function
	// satisfy matrix
	satisfyDeadline := make([][]float64, n)
	for i := 0; i < n; i++ {
		satisfyDeadline[i] = make([]float64, S)
		for j := 0; j < S; j++ {
			if packetsDeadline[j] >= pathDelay[i] {
				satisfyDeadline[i][j] = 1
			}
		}
	}
	// transform to one-dimension vector
	C := make([]float64, S*n)
	for i := 0; i < n; i++ {
		for j := 0; j < S; j++ {
			C[i*S+j] = satisfyDeadline[i][j]
		}
	}

	fmt.Println("C:", C)
	// Solve
	lp := golp.NewLP(0, S*n)
	lp.SetObjFn(C)
	lp.SetMaximize()

	for i := 0; i < len(A); i++ {
		lp.AddConstraint(A[i], golp.LE, b[i])
	}
	fmt.Println("A:", A, "b:", b)

	lp.Solve()
	vars := lp.Variables()

	fmt.Println("vars:", vars)

	result := make([][]float64, S)
	for i := range result {
		result[i] = make([]float64, n)
		for j := range result[i] {
			result[i][j] = vars[i+j*S]
		}
	}
	fmt.Println("result:", result)
	// Convert solution to policy
	policy = resultToPolicy(result)
	return policy
}

func resultToPolicy(result [][]float64) []int {
	// TODO:just adapt to two paths
	policy := make([]int, len(result))
	for i := 0; i < len(result); i++ {
		if len(result[i]) == 1 {
			if result[i][0] == 0 {
				policy[i] = 0
			} else {
				policy[i] = 1
			}
		} else if len(result[i]) == 2 {
			if result[i][0] == 0 && result[i][1] == 0 {
				policy[i] = 0
			} else if result[i][0] != 0 {
				policy[i] = 1
			} else {
				policy[i] = 2
			}
		} else {
			fmt.Println("TODO:just for one path and two path!")
		}
	}
	return policy
}

//rounding

// GenerateBatchDeadline Generator of batch transfer
// TODO:add min/max value interface
func GenerateBatchDeadline(size int) []int {
	deadline := make([]int, size)
	for i := 0; i < size; i++ {
		//randNum := rand.Intn(50)
		randNum := rand.Intn(30) + 10 //10-40 ms
		deadline[i] = randNum
	}
	return deadline
}

// select path for batch packet
func (sch *scheduler) selectBatchPath(s *session, hasRetransmission bool,
	hasStreamRetransmission bool, fromPth *path, deadlineBatch []int) []*path {
	// XXX Currently round-robin
	// TODO:BatchLinOpt do not realize
	if sch.SchedulerName == "BatchLinOpt" {
		fmt.Println("Batch Scheduler: BatchLinOpt")
		return sch.selectBatchlinOpt(s, hasRetransmission, hasStreamRetransmission, fromPth, deadlineBatch)
	} else {
		// Default, all select first path
		fmt.Println("Batch Scheduler: default--First path")
		return sch.selectBatchFirstPath(s, hasRetransmission, hasStreamRetransmission, fromPth, deadlineBatch)
	}
}

func (sch *scheduler) selectBatchFirstPath(s *session,
	hasRetransmission bool, hasStreamRetransmission bool,
	fromPth *path, deadlineBatch []int) []*path {
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		paths := make([]*path, len(deadlineBatch))
		for i := range paths {
			paths[i] = s.paths[protocol.InitialPathID]
		}
		return paths
	}

	// some information
	for pathID, path := range s.paths {
		fmt.Println("pathID:", pathID)
		fmt.Println("CWND:", path.sentPacketHandler.GetCongestionWindow())
		fmt.Println("Inflight Bytes:", path.sentPacketHandler.GetBytesInFlight())
		fmt.Println("RTT:", path.rttStats.SmoothedRTT())
	}

	paths := make([]*path, len(deadlineBatch))
	// a flag
	canSend := false
	for i := 0; i < len(deadlineBatch); i++ {
		for pathID, pth := range s.paths {
			if pathID == protocol.PathID(1) && pth.SendingAllowed() {
				paths[i] = pth
				canSend = true
				break
			}
		}
	}

	if canSend {
		return paths
	} else {
		return nil
	}
}

func (sch *scheduler) selectBatchlinOpt(s *session,
	hasRetransmission bool, hasStreamRetransmission bool,
	fromPth *path, deadlineBatch []int) []*path {
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		paths := make([]*path, len(deadlineBatch))
		for i := range paths {
			paths[i] = s.paths[protocol.InitialPathID]
		}
		return paths
	}

	// Create a slice to store the eligible paths
	eligiblePaths := []*path{}

	// Iterate over the paths and filter out the initial path
	for pathID, pth := range s.paths {
		if pathID == protocol.InitialPathID {
			continue
		} else {
			pthCwnd := pth.sentPacketHandler.GetCongestionWindow() // notice this Cwnd is bytes
			pthInflight := pth.sentPacketHandler.GetBytesInFlight()
			// path that has remaining cwnd is available
			if pthCwnd >= pthInflight {
				eligiblePaths = append(eligiblePaths, pth)
			}
		}
	}

	// Collect the path one way delays and CWNDs
	pathDelays := make([]float64, len(eligiblePaths))
	pathCWNDs := make([]float64, len(eligiblePaths))
	for i, pth := range eligiblePaths {
		//pathDelays[i] = (float64(pth.rttStats.SmoothedRTT()) / float64(time.Millisecond)) / 2
		tempPathDelays := (float64(pth.rttStats.SmoothedRTT()) / float64(time.Millisecond)) / 2
		if banditAvaiable {
			pathDelays[i] = tempPathDelays * float64(pth.sentPacketHandler.GetPathAlpha())
		} else {
			pathDelays[i] = tempPathDelays
		}
		fmt.Println("slect arm pathID:", pth.pathID, ", alpha:", pth.sentPacketHandler.GetPathAlpha())
		remainingCwnd := pth.sentPacketHandler.GetCongestionWindow() - pth.sentPacketHandler.GetBytesInFlight()
		// TODO:remainingCwnd / protocol.MaxPacketSize is a uint64
		pathCWNDs[i] = float64(remainingCwnd / protocol.MaxPacketSize)
	}

	// exceptional situation
	if len(eligiblePaths) == 0 {
		return nil
	}

	packetsNum := generateSequence(len(deadlineBatch))
	packetsDeadline := convertToIntSlice(deadlineBatch)
	fmt.Println("pathOneWayDealys:", pathDelays)
	fmt.Println("pathCWNDs:", pathCWNDs)

	policy := linOpt(packetsNum, packetsDeadline, pathDelays, pathCWNDs)
	paths := PolicyToSelectPath(policy, eligiblePaths)
	fmt.Println("paketsDeadline:", packetsDeadline)
	fmt.Println("policy:", policy)
	return paths
}

func linOptPathCost(packetsNum []int, packetsDeadline []float64, packetsPriority []float64, pathDelay []float64, pathCwnd []float64, pathCost []float64, budgetConstraint float64) []int {
	S := len(packetsNum)     // num of packets
	n := len(pathDelay)      // num of path
	policy := make([]int, S) // scheduler decision

	// Constraint coefficient matrix
	A := make([][]float64, S+n)
	for i := range A {
		A[i] = make([]float64, S*n)
	}
	for i := 0; i < S; i++ {
		for j := i; j < S*n; j += S {
			A[i][j] = 1
		}
	}
	for i := 0; i < n; i++ {
		for j := (i * S); j < ((i + 1) * S); j++ {
			A[S+i][j] = 1
		}
	}

	b := make([]float64, S+n)
	for i := 0; i < S; i++ {
		b[i] = 1
	}
	copy(b[S:], pathCwnd)

	// Objective function
	satisfyDeadline := make([]float64, 2*S)
	for i := 0; i < S; i++ {
		satisfyDeadline[i] = 0
		if packetsDeadline[i] >= pathDelay[0] {
			satisfyDeadline[i] = 1
		}
		if packetsDeadline[i] >= pathDelay[1] {
			satisfyDeadline[i+S] = 1
		}
	}
	C := make([]float64, S*n)
	for i := 0; i < S*n; i++ {
		C[i] = satisfyDeadline[i]
	}

	fmt.Println("C:", C)
	// Solve
	lp := golp.NewLP(0, S*n)
	lp.SetObjFn(C)
	lp.SetMaximize()

	for i := 0; i < len(A); i++ {
		lp.AddConstraint(A[i], golp.LE, b[i])
	}
	fmt.Println("A:", A, "b:", b)

	lp.Solve()
	vars := lp.Variables()
	fmt.Println("vars:", vars)
	result := make([][]float64, S)
	for i := range result {
		result[i] = make([]float64, n)
		for j := range result[i] {
			result[i][j] = vars[i+j*S]
		}
	}

	// Convert solution to policy
	for i := 0; i < len(result); i++ {
		if result[i][0] == 0 && result[i][1] == 0 {
			policy[i] = 0
		} else if result[i][0] != 0 {
			policy[i] = 1
		} else {
			policy[i] = 2
		}
	}

	return policy
}

func generateSequence(length int) []int {
	sequence := make([]int, length)
	for i := 0; i < length; i++ {
		sequence[i] = i + 1
	}
	return sequence
}

func convertToIntSlice(input []int) []float64 {
	converted := make([]float64, len(input))
	for i, num := range input {
		converted[i] = float64(num)
	}
	return converted
}

func PolicyToSelectPath(policy []int, eligiblePath []*path) []*path {
	var selectedPaths []*path

	for _, p := range policy {
		if p == 0 {
			selectedPaths = append(selectedPaths, nil)
		} else if p <= len(eligiblePath) {
			selectedPaths = append(selectedPaths, eligiblePath[p-1])
		} else {
			selectedPaths = append(selectedPaths, nil)
		}
	}

	return selectedPaths
}

func (sch *scheduler) canMadeDecision(s *session, batch int) bool {
	// Create a slice to store the eligible paths
	fmt.Println("Enter can MadeDecision!")
	eligiblePaths := []*path{}
	for pathID, pth := range s.paths {
		if pathID == protocol.InitialPathID {
			continue
		} else {
			fmt.Println("has other path")
			pthCwnd := pth.sentPacketHandler.GetCongestionWindow() // notice this Cwnd is bytes
			pthInflight := pth.sentPacketHandler.GetBytesInFlight()
			// path that has remaining cwnd is available
			if pthCwnd >= pthInflight {
				eligiblePaths = append(eligiblePaths, pth)
			}
		}
	}

	// Collect all the path CWNDs
	var allPathCwnds int

	for _, pth := range eligiblePaths {
		remainingCwnd := pth.sentPacketHandler.GetCongestionWindow() - pth.sentPacketHandler.GetBytesInFlight()
		fmt.Println("remainCwnds:", remainingCwnd)
		// TODO:remainingCwnd / protocol.MaxPacketSize is a uint64
		allPathCwnds = allPathCwnds + int(remainingCwnd/protocol.MaxPacketSize)
		fmt.Println("allPathCwnds:", allPathCwnds)
	}
	if allPathCwnds >= batch {
		return true
	} else {
		return false
	}
}
