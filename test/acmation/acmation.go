package main

import (
	"log"
)

type Trie struct {
	ch    rune
	fail  *Trie
	nexts map[rune]*Trie
	count int
	leaf  bool
	word  string
}

func NewTrie(ch rune) *Trie {
	return &Trie{
		ch:    ch,
		nexts: make(map[rune]*Trie),
	}
}

func (t *Trie) Insert(word string) {
	p := t

	for _, ch := range word {
		if next, ok := p.nexts[ch]; !ok {
			next := NewTrie(ch)
			p.nexts[ch] = next
			p = next
		} else {
			next.count++
			p = next
		}
	}

	p.leaf = true
	p.word = word
}

func (t *Trie) StartsWith(prefix string) bool {
	p := t
	for _, ch := range prefix {
		next, ok := p.nexts[ch]
		if !ok {
			return false
		}

		p = next
	}

	return true
}

func (t *Trie) acmation() { // 构造fail指针
	queue := []*Trie{t}
	for len(queue) > 0 { // bfs遍历每个节点
		tmp := queue[0]
		queue = queue[1:]
		for ch, k := range tmp.nexts {
			if tmp == t { // 根的子节点的fail指向根自己
				k.fail = t
			} else {
				p := tmp.fail // 转到fail
				for p != nil {
					if next, ok := p.nexts[ch]; ok {
						k.fail = next // 节点k的值在子节点中，将k的fail指向对应的子节点
						break
					}
					p = p.fail // 迭代指向下一个fail，继续查找
				}

				if p == nil { // 表示当前节点值在之前没有出现过，则fail指向根
					k.fail = t
				}
			}

			queue = append(queue, k)
		}
	}
}

func (t *Trie) kmp(mode string) {
	p := t

	for _, ch := range mode {
		for { // ch在p.nexts中则直接进入指向的子节点
			_, ok := p.nexts[ch]
			if !ok && p != t {
				p = p.fail // 否则直接跳到fail节点
				continue
			}

			break
		}

		// 经过fail节点的处理后，p不可能为nil
		if next, ok := p.nexts[ch]; ok { // ch在p.nexts中则直接进入指向的子节点
			p = next
		} else {
			p = t // 否则指向根
		}

		tmp := p
		for tmp != t { // 遍历每一个fail节点，若是叶子节点则输出答案
			if tmp.leaf {
				log.Println(tmp.word)
			}
			tmp = tmp.fail
		}
	}
}

func main() {
	root := NewTrie(-1)

	words := []string{"she", "a", "shed", "girl", "ir"}
	for _, word := range words {
		root.Insert(word)
	}

	log.Println(root.StartsWith("sh"))

	root.acmation()

	root.kmp("she is a girl girl")
}
