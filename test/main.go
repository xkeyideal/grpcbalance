package main

import (
	"log"

	"github.com/xkeyideal/grpcbalance/grpclient/priorityqueue"
)

func main() {
	scCostTime := priorityqueue.NewPriorityQueue()

	for i := 0; i < 4; i++ {
		scCostTime.PushItem(&priorityqueue.Item{
			Val:   0,
			Index: i,
		})
	}

	n := scCostTime.Min().(*priorityqueue.Item)
	log.Println(n.Index, n.Val)

	n.Val = 50
	scCostTime.UpdateItem(n)
	n = scCostTime.Min().(*priorityqueue.Item)
	log.Println(n.Index, n.Val)

	n.Val = 20
	scCostTime.UpdateItem(n)
	n = scCostTime.Min().(*priorityqueue.Item)
	log.Println(n.Index, n.Val)

	n.Val = 40
	scCostTime.UpdateItem(n)
	n = scCostTime.Min().(*priorityqueue.Item)
	log.Println(n.Index, n.Val)

	n.Val = 10
	scCostTime.UpdateItem(n)
	n = scCostTime.Min().(*priorityqueue.Item)
	log.Println(n.Index, n.Val)

	n.Val = 20
	scCostTime.UpdateItem(n)
	n = scCostTime.Min().(*priorityqueue.Item)
	log.Println(n.Index, n.Val)
}
