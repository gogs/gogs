package diffmatchpatch

import (
	"fmt"
)

type Stack struct {
	top  *Element
	size int
}

type Element struct {
	value interface{}
	next  *Element
}

// Len returns the stack's length
func (s *Stack) Len() int {
	return s.size
}

// Push appends a new element onto the stack
func (s *Stack) Push(value interface{}) {
	s.top = &Element{value, s.top}
	s.size++
}

// Pop removes the top element from the stack and return its value
// If the stack is empty, return nil
func (s *Stack) Pop() (value interface{}) {
	if s.size > 0 {
		value, s.top = s.top.value, s.top.next
		s.size--
		return
	}
	return nil
}

// Peek returns the value of the element on the top of the stack
// but don't remove it. If the stack is empty, return nil
func (s *Stack) Peek() (value interface{}) {
	if s.size > 0 {
		value = s.top.value
		return
	}
	return -1
}

// Clear empties the stack
func (s *Stack) Clear() {
	s.top = nil
	s.size = 0
}

func main() {
	stack := new(Stack)

	stack.Push("Things")
	stack.Push("and")
	stack.Push("Stuff")

	for stack.Len() > 0 {
		fmt.Printf("%s ", stack.Pop().(string))
	}
	fmt.Println()
}
