package reminder

import "container/heap"

type reminderQueue struct {
	backingArray []*Reminder         // pointer to an element in reminders
	reminders    map[int64]*Reminder // actual reminders
}

func NewReminderQueue() *reminderQueue {
	r := &reminderQueue{
		backingArray: []*Reminder{},
		reminders:    make(map[int64]*Reminder),
	}
	heap.Init(r)
	return r
}

func (rq reminderQueue) Len() int {
	return len(rq.backingArray)
}

func (rq reminderQueue) Less(i, j int) bool {
	return rq.backingArray[i].at.Unix() < rq.backingArray[j].at.Unix()
}

func (rq reminderQueue) Swap(i, j int) {
	rq.backingArray[j], rq.backingArray[i] = rq.backingArray[i], rq.backingArray[j]
}

func (rq *reminderQueue) Push(r any) {
	reminder, ok := r.(*Reminder)
	if !ok {
		return
	}

	// first save the reminder, then save a pointer to it
	rq.reminders[reminder.usr] = reminder
	rq.backingArray = append(rq.backingArray, reminder)
}

func (rq *reminderQueue) Pop() any {
	if len(rq.backingArray) == 0 {
		return nil
	}

	ba := rq.backingArray
	n := len(ba)
	rq.backingArray = ba[:n-1]
	popped := ba[n-1]
	r := rq.reminders[popped.usr]
	delete(rq.reminders, popped.usr)

	return r
}

func (rq *reminderQueue) Delete(usr int64) {
	delete(rq.reminders, usr)
}

func (rq *reminderQueue) Peek() any {
	if len(rq.backingArray) == 0 {
		return nil
	}

	return rq.reminders[rq.backingArray[0].usr]
}
