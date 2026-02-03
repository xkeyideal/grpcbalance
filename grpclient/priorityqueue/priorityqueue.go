package priorityqueue

import (
	"container/heap"
)

type Item struct {
	Addr       string
	Val        int64
	Index      int
	queueIndex int
}

func NewPriorityQueue() *PriorityQueue {
	queue := &PriorityQueue{}
	heap.Init(queue)
	return queue
}

type PriorityQueue struct {
	items []*Item
}

func (pq *PriorityQueue) UpdateItem(item *Item) {
	heap.Fix(pq, item.queueIndex)
}

func (pq *PriorityQueue) PushItem(item *Item) {
	heap.Push(pq, item)
}

func (pq *PriorityQueue) PopItem() *Item {
	if pq.Len() == 0 {
		return nil
	}
	return heap.Pop(pq).(*Item)
}

func (pq *PriorityQueue) RemoveItem(item *Item) {
	heap.Remove(pq, item.queueIndex)
}

func (pq PriorityQueue) Len() int {
	length := len(pq.items)
	return length
}

func (pq PriorityQueue) Less(i, j int) bool {
	return pq.items[i].Val < pq.items[j].Val
}

func (pq PriorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
	pq.items[i].queueIndex = i
	pq.items[j].queueIndex = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	item := x.(*Item)
	item.queueIndex = len(pq.items)
	pq.items = append(pq.items, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := pq.items
	n := len(old)
	item := old[n-1]
	item.queueIndex = -1
	pq.items = old[0 : n-1]
	return item
}

func (pq *PriorityQueue) Min() interface{} {
	if len(pq.items) == 0 {
		return nil
	}
	return pq.items[0]
}
