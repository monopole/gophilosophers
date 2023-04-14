/*
  The "Dining Philosophers" problem modelled using Go channels.

    A circular table has N philosophers and N chopstick trays,
    with a single stick in each tray.

    Each stick is shared by two adjacent philosophers.

    There's a rice bowl in the middle.

    A philosopher's life cycle is
     - grab two chopsticks (the number required for eating),
     - eat one serving of rice from the bowl,
     - release the chopsticks
     - think,
     - repeat the above until there is no more food,
       ending the simulation.

  A simulation is controlled by some constants listed below
  (NumPhilosophers, ThingingDuration, NumServings, etc).

  - Every philosopher is a go routine.
  - The rice bowl is a channel of servings.
  - The chopsticks are objects that can collect stats about their use.
  - The trays are channels to hand sticks back and forth.
*/

package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

const (
	// NumPhilosophers is how many philosophers.
	// Increase this to increase contention.
	NumPhilosophers = 200

	// ThinkingDuration is how long a philosopher thinks after releasing chopsticks,
	// before attempting to eat again.
	// Decrease this to increase contention.
	ThinkingDuration = 3 * time.Millisecond

	// NumServings is a count of the number of servings of rice in the bowl
	// in the center of the diningTable. Increase this to run longer.
	// When a philosopher eats, they consume one serving and release their chopsticks.
	// When a philosopher finds that no servings are left, he leaves the table.
	// When all have left, the program terminates.
	// Increase this to assure everyone gets to eat.
	// Starvation is assured if NumServings < NumPhilosophers.
	NumServings = 199
)

type serving struct{}

type chopStick struct {
	id        int
	countGrab int
	countEat  int
}

type riceBowl chan serving

// stickTray can hold a chopstick in a channel buffer of size 1.
// To ease reporting and statistics, it knows the philosopher to its left and right.
type stickTray struct {
	ch    chan *chopStick
	left  *philosopher
	right *philosopher
}

// philosopher records stats and holds pointers to things to simplify the code
// and ease reporting.
// There's no need to actually hold the chopsticks, but it helps
// with the reporting.
type philosopher struct {
	id                 int
	hadToWaitCount     int
	servingsEatenCount int
	// the philosopher's hands can hold a chopstick (or nil)
	handLeft  *chopStick
	handRight *chopStick
	// Pointer to the left and right trays are held here to simplify code.
	// These are never nil once initialized.
	trayLeft  *stickTray
	trayRight *stickTray
}

func (p *philosopher) dump() {
	fmt.Printf("philosopher%3d waited%4d times, ate%4d times  ",
		p.id, p.hadToWaitCount, p.servingsEatenCount)
	if p.servingsEatenCount == 0 {
		fmt.Printf(" STARVED!")
	}
	fmt.Println()
}

// seat groups a philosopher, an actual chopStick, and a tray to put the stick in
// when it's not being used to eat.  The trouble is that each philosopher must
// share their stick with their neighbor.
// The stick is an actual thing because it's used to collect stats.
type seat struct {
	diner philosopher
	tray  stickTray
	stick chopStick
}

// diningTable arranges N seats in a ring.
type diningTable []seat

// Indented philosopher id, to facilitate reading interleaved go routine output.
func (p *philosopher) sid() string {
	return fmt.Sprintf("%*sp%d", 2*(p.id+1), " ", p.id)
}

func (p *philosopher) eat() {
	p.handLeft.countEat++
	p.handRight.countEat++
	p.servingsEatenCount++
	fmt.Printf("%s eats!\n", p.sid())
}

func (p *philosopher) think() {
	fmt.Printf("%s has eaten %d bites; starting to think.\n", p.sid(), p.servingsEatenCount)
	time.Sleep(ThinkingDuration)
	fmt.Printf("%s done thinking.\n", p.sid())
}

func (p *philosopher) releaseLeft(why string) {
	fmt.Printf("%s releases stick %d; %s.\n", p.sid(), p.handLeft.id, why)
	p.trayLeft.ch <- p.handLeft
	p.handLeft = nil
}

func (p *philosopher) releaseRight(why string) {
	fmt.Printf("%s releases stick %d; %s.\n", p.sid(), p.handRight.id, why)
	p.trayRight.ch <- p.handRight
	p.handRight = nil
}

// grabSticks grabs two sticks - the one from the left tray and the one from the right tray
func (p *philosopher) grabSticks() {
	tries := 0
	for {
		select {
		case p.handLeft = <-p.trayLeft.ch:
			p.handLeft.countGrab++
			fmt.Printf("%s takes stick %d from left.\n", p.sid(), p.handLeft.id)
			select {
			case p.handRight = <-p.trayRight.ch:
				p.handRight.countGrab++
				fmt.Printf("%s takes stick %d from right; now has both (%d tries).\n", p.sid(), p.handRight.id, tries)
				return
			default:
				p.releaseLeft("got left, but unable to get right")
			}

		case p.handRight = <-p.trayRight.ch:
			p.handRight.countGrab++
			fmt.Printf("%s takes stick %d from right.\n", p.sid(), p.handRight.id)
			select {
			case p.handLeft = <-p.trayLeft.ch:
				p.handLeft.countGrab++
				fmt.Printf("%s takes stick %d from left; now has both (%d tries).\n", p.sid(), p.handLeft.id, tries)
				return
			default:
				p.releaseRight("got right, but unable to get left")
			}
		}
		p.hadToWaitCount++
		tries++
		fmt.Printf("%s unable to get chopsticks in %d consecutive attempts.\n", p.sid(), tries)
	}
}

func (p *philosopher) releaseSticks(msg string) {
	p.releaseLeft(msg)
	p.releaseRight(msg)
}

func (p *philosopher) eatAndThink(bowl riceBowl, wait *sync.WaitGroup) {
	for {
		p.grabSticks()
		// Take a serving
		if _, ok := <-bowl; !ok {
			// No more food, time to leave.
			p.releaseSticks("no more food")
			wait.Done()
			return
		}
		p.eat()
		p.releaseSticks("ate one serving")
		p.think()
	}
}

// makeDiningTable returns a table of philosophers separated by trays.
func makeDiningTable(numPhilosophers int) diningTable {
	tuples := make(diningTable, numPhilosophers)
	// Make everything.
	for i := range tuples {
		tuples[i].diner.id = i
		// Using no buffer below would mean that a chopstick could not be put down until someone was waiting
		// to pick it up.  That would be a problem, since philosophers sometimes think rather than try to eat.
		// Using a buffer of size '1' here means it's possible to put a chopstick down if nobody is waiting.
		// Using a larger buffer just wastes space.
		tuples[i].tray.ch = make(chan *chopStick, 1)
		tuples[i].stick.id = i
	}
	leftI := func(i int) int {
		return (numPhilosophers + i - 1) % numPhilosophers
	}
	rightI := func(i int) int {
		return (i + 1) % numPhilosophers
	}
	// Hook everything up.
	for i := range tuples {
		philosopher := &(tuples[i].diner)
		philosopher.trayLeft = &tuples[leftI(i)].tray
		philosopher.trayRight = &tuples[i].tray
		tray := &(tuples[i].tray)
		tray.left = &tuples[i].diner
		tray.right = &tuples[rightI(i)].diner
	}
	return tuples
}

func (dt diningTable) report() {
	fmt.Println("\nReport:")
	for i := range dt {
		dt[i].diner.dump()
	}
	for i := range dt {
		fmt.Printf("stick%3d grabbed%4d times, used to eat%4d times\n",
			i, dt[i].stick.countGrab, dt[i].stick.countEat)
	}
}

func (dt diningTable) placeChopsticksInTrays() {
	for i := range dt {
		fmt.Printf("Placing chopstick %d\n", i)
		dt[i].tray.ch <- &dt[i].stick
	}
}

// serveDinner makes a table, starts everyone eating, and waits till they are all done.
func (dt diningTable) serveDinner(numServings int) {
	// Start philosophers - though they won't be able to eat till the sticks are out and rice is served.
	// Each philosopher expected to eat from the bowl until
	// the bowl is empty, then signal their completion on the WaitGroup.
	bowl := make(riceBowl, numServings)
	var wait sync.WaitGroup
	wait.Add(NumPhilosophers)
	for i := range dt {
		go dt[i].diner.eatAndThink(bowl, &wait)
	}

	fmt.Printf("Philosophers started, numGoroutine = %d\n", runtime.NumGoroutine())

	// Unblock everyone, but there's still nothing to eat.
	dt.placeChopsticksInTrays()
	// Now serve the rice.
	go serveRice(bowl, numServings)
	// Wait for everyone to finish eating all the servings.
	wait.Wait()

	dt.report()
}

// serveRice just fills a channel with servings and closes it.
// The channel assures only one diner can eat a serving.
// Allows accurate total consumption count.
// Since this is just a counter decrement, could model it as a semaphore protected int,
// but goal here is to use only channels for synchronization.
func serveRice(ch riceBowl, numServings int) {
	for i := 0; i < numServings; i++ {
		ch <- serving{}
	}
	close(ch)
}

func grabAllCpus() {
	numCpus := runtime.NumCPU()
	fmt.Printf("num cpus = %d\n", numCpus)
	runtime.GOMAXPROCS(numCpus)
	fmt.Printf("max cpus = %d\n", runtime.GOMAXPROCS(numCpus))
	fmt.Printf("Before any 'go' starts, numGoroutine = %d\n", runtime.NumGoroutine())
}

func main() {
	fmt.Printf("version = %s\n", runtime.Version())
	if NumPhilosophers < 2 {
		fmt.Printf("Indexing scheme demands at least 2 philosophers.\n")
		return
	}
	// Someone might starve even if NumServings > NumPhilosophers.
	if NumServings < NumPhilosophers {
		fmt.Printf("Starvation certain.\n")
	}
	grabAllCpus()
	table := makeDiningTable(NumPhilosophers)
	table.serveDinner(NumServings)
	fmt.Printf("All done.\n")
}
