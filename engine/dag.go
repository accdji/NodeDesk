package engine

import "fmt"

type DAG map[string][]string

func stepID(s *Plugin) string {
	if s.StepID != "" {
		return s.StepID
	}
	return s.Name
}

func BuildDAG(steps []*Plugin) DAG {
	dag := make(DAG)
	ids := make(map[string]bool)
	for _, s := range steps {
		id := stepID(s)
		ids[id] = true
		if _, ok := dag[id]; !ok {
			dag[id] = []string{}
		}
	}
	for _, s := range steps {
		id := stepID(s)
		for _, dep := range s.DependsOn {
			if ids[dep] {
				dag[dep] = append(dag[dep], id)
			}
		}
	}
	return dag
}

func TopologicalSort(steps []*Plugin, dag DAG) ([]string, error) {
	// 先检测是否有环，返回具体路径
	if hasCycle, cycle := DetectCycle(dag); hasCycle {
		cycleStr := ""
		for i, name := range cycle {
			if i > 0 {
				cycleStr += " → "
			}
			cycleStr += name
		}
		return nil, fmt.Errorf("DAG 循环依赖: %s", cycleStr)
	}

	inDegree := make(map[string]int)
	for _, s := range steps {
		inDegree[stepID(s)] = 0
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
		return nil, fmt.Errorf("DAG 拓扑排序失败: %d/%d 节点已排序", len(order), len(steps))
	}
	return order, nil
}

// DetectCycle 检测 DAG 中是否存在循环依赖，返回环路路径
func DetectCycle(dag DAG) (bool, []string) {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	parent := make(map[string]string)

	var dfs func(node string) (bool, []string)
	dfs = func(node string) (bool, []string) {
		visited[node] = true
		recStack[node] = true
		for _, neighbor := range dag[node] {
			parent[neighbor] = node
			if !visited[neighbor] {
				if found, cycle := dfs(neighbor); found {
					return true, cycle
				}
			} else if recStack[neighbor] {
				// 找到环，回溯构建路径
				cycle := []string{neighbor, node}
				cur := node
				for cur != neighbor && parent[cur] != "" {
					cur = parent[cur]
					cycle = append(cycle, cur)
				}
				// 反转使路径从起点开始
				for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
					cycle[i], cycle[j] = cycle[j], cycle[i]
				}
				return true, cycle
			}
		}
		recStack[node] = false
		return false, nil
	}

	for node := range dag {
		if !visited[node] {
			if found, cycle := dfs(node); found {
				return true, cycle
			}
		}
	}
	// 也检查没有出边的节点
	for node := range dag {
		for _, neighbor := range dag[node] {
			if !visited[neighbor] {
				if found, cycle := dfs(neighbor); found {
					return true, cycle
				}
			}
		}
	}
	return false, nil
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
