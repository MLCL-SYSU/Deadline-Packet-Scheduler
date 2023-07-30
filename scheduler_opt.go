package quic

import (
	"fmt"
	"github.com/draffensperger/golp"
	"github.com/lucas-clemente/quic-go/internal/protocol"
	"math/rand"
	"sort"
	"time"
)

// some parameter
const banditAvailable = false
const costConstraintAvailable = false
const path1Cost = 1.5 //cellular link
const path3Cost = 0.5 //WiFi link
const budget = 4
const alpha1 = 1.1
const alpha2 = 1.2

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

func linOptCost(packetsNum []int, packetsDeadline []float64, pathDelay []float64,
	pathCwnd []float64, pathCost []float64, budgetConstraint float64) []int {
	// TODO:packetsNum is unnecessary
	S := len(packetsNum)     // num of packets
	n := len(pathDelay)      // num of path
	policy := make([]int, S) // scheduler decision: [1 2] means packet 1 schedule in path 1, packet 2 schedule in path 2

	// Constraint coefficient matrix
	A := make([][]float64, S+n+1) // add one cost constraint
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

	for i := 0; i < S*n; i++ {
		A[S+n][i] = pathCost[i/S]
	}

	b := make([]float64, S+n+1)
	for i := 0; i < S; i++ {
		b[i] = 1
	}
	copy(b[S:], pathCwnd)
	b[S+n] = budgetConstraint

	fmt.Println("A:", A, "b:", b)

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
	lp := golp.NewLP(0, S*n+1)
	lp.SetObjFn(C)
	lp.SetMaximize()

	for i := 0; i < len(A); i++ {
		lp.AddConstraint(A[i], golp.LE, b[i])
	}

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
	policy = resultToPolicyWithGreedyRounding(result, S, n)
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

//Greedy Rounding
func resultToPolicyWithGreedyRounding(result [][]float64, S int, n int) []int {
	policy := make([]int, len(result))

	for i := 0; i < S; i++ {
		probSort := make([]float64, n)
		copy(probSort, result[i])

		sort.Slice(probSort, func(j, k int) bool {
			return probSort[j] > probSort[k]
		})

		fmt.Println("probSort:", probSort)

		for j := 0; j < n; j++ {
			if probSort[j] == 1 {
				index := findIndex(result[i], probSort[j])
				policy[i] = index + 1
				break
			} else {
				if result[i][j] != 0 && policy[i] == 0 {
					policyCandidate := []int{j + 1, 0} // rounding到第j个path或不发
					prob := []float64{result[i][j], 1 - result[i][j]}
					policy[i] = chooseByProb(policyCandidate, prob)
				}
			}
		}
	}

	return policy
}

func findIndex(arr []float64, target float64) int {
	for i, val := range arr {
		if val == target {
			return i
		}
	}
	return -1
}

// chooseByProb choose a value by probability
func chooseByProb(value []int, Prob []float64) int {
	r := rand.Float64()
	sum := 0.0
	for i, p := range Prob {
		sum += p
		if r < sum {
			return value[i]
		}
	}
	return value[len(value)-1]
}

// GenerateBatchDeadline Generator of batch transfer
// TODO:add min/max value interface
func (sch *scheduler) GenerateBatchDeadline(size int, curTime time.Time) []int {
	lenWait := len(sch.waitPackets)
	Deadline := make([]int, size-lenWait)
	for i := 0; i < size-lenWait; i++ {
		//randNum := rand.Intn(50)
		randNum := rand.Intn(30) + 10 //10-40 ms
		Deadline[i] = randNum
	}
	for _, deadlineTime := range sch.waitPackets {
		durationTime := deadlineTime.Sub(curTime)
		Deadline = append(Deadline, int(durationTime.Milliseconds()))
		fmt.Println("new Deadline:", int(durationTime.Milliseconds()))
	}
	sch.waitPackets = make([]time.Time, 0)
	return Deadline
}

// select path for batch packet
func (sch *scheduler) selectBatchPath(s *session, hasRetransmission bool,
	hasStreamRetransmission bool, fromPth *path, deadlineBatch []int) []*path {
	// XXX Currently round-robin
	// TODO:BatchLinOpt do not realize
	if sch.SchedulerName == "BatchLinOpt" {
		fmt.Println("Batch Scheduler: BatchLinOpt")
		return sch.selectBatchlinOpt(s, hasRetransmission, hasStreamRetransmission, fromPth, deadlineBatch)
	} else if sch.SchedulerName == "BatchEDF" {
		fmt.Println("Batch Scheduler: EDF")
		return sch.selectBatchEDF(s, hasRetransmission, hasStreamRetransmission, fromPth, deadlineBatch)
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
	// cost constraint
	pathCost := make([]float64, len(eligiblePaths))
	for i, pth := range eligiblePaths {
		//pathDelays[i] = (float64(pth.rttStats.SmoothedRTT()) / float64(time.Millisecond)) / 2
		tempPathDelays := (float64(pth.rttStats.SmoothedRTT()) / float64(time.Millisecond)) / 2
		if banditAvailable {
			pathDelays[i] = tempPathDelays * float64(pth.sentPacketHandler.GetPathAlpha())
			//pathDelays[i] = tempPathDelays * alpha1
			//pathDelays[i] = tempPathDelays * alpha2
		} else {
			pathDelays[i] = tempPathDelays
		}

		if pth.pathID == protocol.PathID(1) {
			pathCost[i] = path1Cost
		} else if pth.pathID == protocol.PathID(3) {
			pathCost[i] = path3Cost
		}

		if banditAvailable {
			fmt.Println("select arm pathID:", pth.pathID, ", alpha:", pth.sentPacketHandler.GetPathAlpha())
		}
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

	// linOpt solver, when costConstraintAvailable is true, call linOptCost
	// policy is a 1*batchSize vector
	var policy []int
	if costConstraintAvailable {
		policy = linOptCost(packetsNum, packetsDeadline, pathDelays, pathCWNDs, pathCost, budget)
	} else {
		policy = linOpt(packetsNum, packetsDeadline, pathDelays, pathCWNDs)
	}
	paths := PolicyToSelectPath(policy, eligiblePaths)
	fmt.Println("paketsDeadline:", packetsDeadline)
	fmt.Println("policy:", policy)

	// compute cost
	if costConstraintAvailable {
		cost := computeCost(paths)
		fmt.Println("Current Cost:", cost)
		//sch.totalCost += cost // can not add total cost here, because maybe some packets is not sent
	}
	return paths
}

func (sch *scheduler) selectBatchEDF(s *session,
	hasRetransmission bool, hasStreamRetransmission bool,
	fromPth *path, deadlineBatch []int) []*path {

	// Sort Deadline, will change scheduler.go DeadlineBatch
	sort.Ints(deadlineBatch)

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

	// Iterate and pick out the pathBatch through minRTT
	for i := 0; i < len(deadlineBatch); i++ {
		pth := sch.selectPathLowLatency(s, hasRetransmission, hasStreamRetransmission, fromPth)
		eligiblePaths = append(eligiblePaths, pth)
	}

	return eligiblePaths
}

func computeCost(paths []*path) float64 {
	var cost float64
	for _, pth := range paths {
		if pth != nil {
			if pth.pathID == protocol.PathID(1) {
				cost += path1Cost
			} else if pth.pathID == protocol.PathID(3) {
				cost += path3Cost
			}
		}
	}
	return cost
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
