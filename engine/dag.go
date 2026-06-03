package engine

import "fmt"

type DAG map[string][]string

func BuildDAG(steps []*Plugin) DAG {
	dag := make(DAG)
	names := make(map[string]bool)
	for _, s := range steps {
		names[s.Name] = true
		if _, ok := dag[s.Name]; !ok {
			dag[s.Name] = []string{}
		}
	}
	for _, s := range steps {
		for _, dep := range s.DependsOn {
			if names[dep] {
				dag[dep] = append(dag[dep], s.Name)
			}
		}
	}
	return dag
}

func TopologicalSort(steps []*Plugin, dag DAG) ([]string, error) {
	inDegree := make(map[string]int)
	for _, s := range steps {
		inDegree[s.Name] = 0
	}
	for _, ds := range dag {
		for _, d := range ds {
			inDegree[d]++
		}
	}
	queue := make([]string, 0)
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	order := make([]string, 0, len(steps))
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)
		for _, ds := range dag[node] {
			inDegree[ds]--
			if inDegree[ds] == 0 {
				queue = append(queue, ds)
			}
		}
	}
	if len(order) != len(steps) {
		return nil, fmt.Errorf("DAG 循环依赖: %d/%d 节点已排序", len(order), len(steps))
	}
	return order, nil
}

func GetDownstream(dag DAG, step string) map[string]bool {
	r := make(map[string]bool)
	stack := []string{step}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, ds := range dag[node] {
			if !r[ds] {
				r[ds] = true
				stack = append(stack, ds)
			}
		}
	}
	return r
}
