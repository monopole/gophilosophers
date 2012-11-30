/*
  Dining philosophers modelled using channels.

  A circular table has N philosphers and N chopsticks, each stick shared by two adjacent philosphers.

  There's a rice bowl in the middle.

  A philosphers life cycle is grab two chopsticks, eat one serving of rice from the bowl,
  release the chopsticks, think, then try to eat again - until the bowl has no more food.

  Upon simulation completion, stats are printed for each philsopher and each chopstick.

  Every philospher is a go routine.  The rice bowl is a channel of servings, and each chopstick
  is a channel connecting its adjacent philosphers.

  jeff.regan@gmail.com
*/

package main

import (
	"fmt"
	"runtime"
	"time"
)

const (
	// Increase to increase contention.
	NumPhilosophers = 15

	// How long a philosopher thinks after releasing chopsticks,
	// before attempting to eat again.
	// Decrease to increase contention.
	ThinkingDuration = 1 * time.Millisecond

	// Increase this to run longer.
	// When a philosopher eats, they consume one serving and release their chopsticks.
	// When no servings are left, the philosopher leaves the table.
	// When all have left, program terminates.
	NumServings = 2000
)

type Chopstick struct {
	id    int
	ch    chan bool
	left  *Philosopher
	right *Philosopher
}

type RiceBowl chan bool

type Philosopher struct {
	id         int
	done       bool
	hadToWait  int
	left       *Chopstick
	right      *Chopstick
	bitesEaten int
}

func (p *Philosopher) dump() {
	fmt.Printf("%+v\n", p)
	// fmt.Printf("(p%2d) <- c%2d <-(p%2d)-> c%2d -> (p%2d)\n",
	//	p.left.left.id, p.left.id, p.id, p.right.id, p.right.right.id)
}

// Allows single index loop over all philosphers and chopsticks.
type ModelPair struct {
	philosopher Philosopher
	chopstick   Chopstick
}

// Indented philosopher id, to faciliate reading interleaved go routine output.
func (p *Philosopher) sid() string {
	return fmt.Sprintf("%*sp%d", 2*(p.id+1), " ", p.id)
}

func (p *Philosopher) eat() {
	fmt.Printf("%s is eating\n", p.sid())
}

func (p *Philosopher) think() {
	fmt.Printf("%s has eaten %d bites; starting to think.\n", p.sid(), p.bitesEaten)
	time.Sleep(ThinkingDuration)
	fmt.Printf("%s done thinking.\n", p.sid())
}

func (p *Philosopher) ignoreLeft() {
	fmt.Printf("%s notices left neighbor p%d done, so dropping left chopstick (c%d).\n",
		p.sid(), p.left.left.id, p.left.id)
}

func (p *Philosopher) ignoreRight() {
	fmt.Printf("%s notices right neighbor p%d done, so dropping right chopstick (c%d).\n", p.sid(), p.right.right.id, p.right.id)
}

func (p *Philosopher) releaseLeft(why string) {
	fmt.Printf("%s releases left (c%d); %s.\n", p.sid(), p.left.id, why)
	p.left.ch <- true
}

func (p *Philosopher) releaseRight(why string) {
	fmt.Printf("%s releases right (c%d); %s.\n", p.sid(), p.right.id, why)
	p.right.ch <- true
}

func (p *Philosopher) grabSticks() {
	tries := 0
	for {
		select {
		case <-p.left.ch:
			fmt.Printf("%s takes left (c%d).\n", p.sid(), p.left.id)
			select {
			case <-p.right.ch:
				fmt.Printf("%s takes right (c%d); now has both (%d trys).\n", p.sid(), p.right.id, tries)
				return
			default:
				p.releaseLeft("unable to get right")
			}

		case <-p.right.ch:
			fmt.Printf("%s takes right (c%d).\n", p.sid(), p.right.id)
			select {
			case <-p.left.ch:
				fmt.Printf("%s takes left (c%d); now has both (%d trys).\n", p.sid(), p.left.id, tries)
				return
			default:
				p.releaseRight("unable to get left")
			}
		}
		p.hadToWait++
		tries++
		fmt.Printf("%s unable to get chopsticks in %d consecutive attempts.\n", p.sid(), tries)
	}
}

func (p *Philosopher) releaseSticks(msg string) {
	if p.left.left.done {
		p.ignoreLeft()
	} else {
		p.releaseLeft(msg)
	}
	if p.right.right.done {
		p.ignoreRight()
	} else {
		p.releaseRight(msg)
	}
}

func (p *Philosopher) live(bowl RiceBowl, allDone chan bool) {
	for {
		p.grabSticks()

		// Take a serving
		_, ok := <-bowl
		if !ok {
			// No more food, time to leave.
			fmt.Printf("%s finds no more food, quitting.\n", p.sid())
			p.done = true
			p.releaseSticks("no more food")
			allDone <- true
			return
		}
		fmt.Printf("%s took a bite.\n", p.sid())
		p.bitesEaten++
		p.releaseSticks("ate serving")
		p.think()
	}
}

func dumpModel(pairs []ModelPair) {
	fmt.Println("\nAll philosophers:")
	for i := range pairs {
		pairs[i].philosopher.dump()
	}
}

// Return a slice holding all the philosophers and chopsticks.
func initializeModel(numPhilosophers int) []ModelPair {
	pairs := make([]ModelPair, numPhilosophers)
	// Make everything.
	for i := range pairs {
		pairs[i].philosopher.id = i
		pairs[i].chopstick.id = i
		// Using no buffer below would mean that a chopstick could not be put down until someone was waiting
		// to pick it up.  That would be a problem, since philosphers somethings think rather than try to eat.
		// Using a buffer of size '1' here means it's possible to put a chopstick down if nobody is waiting.
		// Using a larger buffer would just waste space.
		pairs[i].chopstick.ch = make(chan bool, 20)
	}
	leftI := func(i int) int {
		return (numPhilosophers + i - 1) % numPhilosophers
	}
	rightI := func(i int) int {
		return (i + 1) % numPhilosophers
	}
	// Hook everything up.
	for i := range pairs {
		philosopher := &(pairs[i].philosopher)
		philosopher.left = &pairs[leftI(i)].chopstick
		philosopher.right = &pairs[i].chopstick
		chopstick := &(pairs[i].chopstick)
		chopstick.left = &pairs[i].philosopher
		chopstick.right = &pairs[rightI(i)].philosopher
	}
	return pairs
}

// Serve rice; a serving is a bit arbitrarily set true.
// Channel assures only one diner can eat a serving.
// Allows accurate total consumption count.
// Since this is just a counter decrement, likely better to model it
// as a semaphore protected int, but goal here is to use only channels.
func serveRice(ch RiceBowl) {
	for i := 0; i < NumServings; i++ {
		ch <- true
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

// Place chopsticks on the table, unblocking the philosophers.
func releaseChopsticks(pairs []ModelPair) {
	for i := range pairs {
		fmt.Printf("Placing chopstick c%d to right of philosopher p%d.\n", pairs[i].chopstick.id, i)
		pairs[i].chopstick.ch <- true
	}
}

func main() {
	fmt.Printf("version = %s\n", runtime.Version())

	if NumPhilosophers < 2 {
		fmt.Printf("Indexing scheme demands at least 2 philosophers.\n")
		return
	}

	grabAllCpus()

	bowl := make(RiceBowl, NumServings)
	pairs := initializeModel(NumPhilosophers)
	dumpModel(pairs)

	// Start philosophers - though they won't be able to eat till rice is served.
	// Each philosopher expected to eat from the bowl until
	// the bowl is empty, then write 'true' to allDone channel.
	allDone := make(chan bool, NumPhilosophers)
	for i := range pairs {
		go pairs[i].philosopher.live(bowl, allDone)
	}

	fmt.Printf("Philosophers started, numGoroutine = %d\n", runtime.NumGoroutine())

	go serveRice(bowl)

	fmt.Printf("Now serving rice, numGoroutine = %d\n", runtime.NumGoroutine())

	// Unblock everyone.
	releaseChopsticks(pairs)

	// Wait for all philosophers to recognize that food is gone.
	for _ = range pairs {
		<-allDone
	}

	dumpModel(pairs)

	fmt.Printf("All done.\n")
}
