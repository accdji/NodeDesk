import { useRef, useEffect, useState, useCallback } from 'react';
import * as d3 from 'd3';
import type { DagNode, DagEdge, PluginInfo } from '../../types';
import { STATUS_COLORS } from '../../utils/constants';

interface DagCanvasProps {
  nodes: DagNode[];
  edges: DagEdge[];
  selectedNodeId: string | null;
  onNodeClick: (id: string) => void;
  onEdgeCreate?: (from: string, to: string) => void;
  onEdgeDelete?: (from: string, to: string) => void;
  onAddNodeFromPort?: (fromNodeId: string, plugin: PluginInfo) => void;
  plugins?: PluginInfo[];
  onAutoLayout?: () => void;
  readonly?: boolean;
}

const NODE_W = 140;
const NODE_H = 32;
const PORT_R = 4.5;
const H_SPACING = 170;
const V_SPACING = 46;
const PADDING = 60;

const TYPE_COLORS: Record<string, string> = {
  script: '#7C3AED',
  condition: '#F59E0B',
  loop: '#06B6D4',
  start: '#10B981',
  end: '#EF4444',
  mapper: '#8B5CF6',
};

const TYPE_ICONS: Record<string, string> = {
  script: '⬡',
  condition: '◇',
  loop: '↻',
  start: '▶',
  end: '■',
  mapper: '⬢',
};

interface Position {
  x: number;
  y: number;
}

interface ContextMenuState {
  fromNodeId: string;
  clientX: number;
  clientY: number;
}

export function DagCanvas({
  nodes, edges, selectedNodeId, onNodeClick,
  onEdgeCreate, onEdgeDelete, onAddNodeFromPort, plugins, onAutoLayout, readonly,
}: DagCanvasProps) {
  const svgRef = useRef<SVGSVGElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const zoomRef = useRef<d3.ZoomBehavior<SVGSVGElement, unknown>>(null!);
  const mainGroupRef = useRef<SVGGElement | null>(null);
  const positionsRef = useRef<Record<string, Position>>({});
  const dragRef = useRef<{ from: string; sx: number; sy: number } | null>(null);
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);

  const computePositions = useCallback((): Record<string, Position> => {
    if (nodes.length === 0) return {};

    const inDegree: Record<string, number> = {};
    const adj: Record<string, string[]> = {};
    nodes.forEach((n) => { inDegree[n.id] = 0; adj[n.id] = []; });
    edges.forEach((e) => {
      if (adj[e.from]) adj[e.from].push(e.to);
      if (inDegree[e.to] !== undefined) inDegree[e.to]++;
    });

    const levels: string[][] = [];
    let queue = Object.keys(inDegree).filter((id) => inDegree[id] === 0);
    if (queue.length === 0 && nodes.length > 0) queue = [nodes[0].id];

    const levelMap: Record<string, number> = {};
    let levelIdx = 0;
    while (queue.length > 0) {
      levels.push([...queue]);
      queue.forEach((id) => { levelMap[id] = levelIdx; });
      const next: string[] = [];
      queue.forEach((id) => {
        (adj[id] || []).forEach((child) => {
          inDegree[child]--;
          if (inDegree[child] === 0) next.push(child);
        });
      });
      queue = next;
      levelIdx++;
    }

    nodes.forEach((n) => {
      if (!(n.id in levelMap)) {
        levelMap[n.id] = levels.length;
        levels.push([n.id]);
      }
    });

    const positions: Record<string, Position> = {};
    levels.forEach((level, li) => {
      const totalH = level.length * (NODE_H + V_SPACING) - V_SPACING;
      const startY = Math.max(PADDING, 100 - totalH / 2);
      level.forEach((nodeId, ni) => {
        const existingNode = nodes.find((n) => n.id === nodeId);
        if (existingNode?._x !== undefined && existingNode?._y !== undefined) {
          positions[nodeId] = { x: existingNode._x!, y: existingNode._y! };
        } else {
          positions[nodeId] = {
            x: PADDING + li * H_SPACING,
            y: startY + ni * (NODE_H + V_SPACING),
          };
        }
      });
    });

    return positions;
  }, [nodes, edges]);

  const buildEdgePath = (x1: number, y1: number, x2: number, y2: number): string => {
    const dx = Math.abs(x2 - x1);
    const cp = Math.min(dx * 0.45, 100);
    return `M${x1},${y1} C${x1 + cp},${y1} ${x2 - cp},${y2} ${x2},${y2}`;
  };

  // Main D3 render
  useEffect(() => {
    if (!svgRef.current || nodes.length === 0) return;

    const svg = d3.select(svgRef.current);
    svg.selectAll('*').remove();

    const pos = computePositions();
    positionsRef.current = pos;

    // Compute SVG size
    let maxX = 400, maxY = 200;
    Object.values(pos).forEach((p) => {
      maxX = Math.max(maxX, p.x + NODE_W + PADDING);
      maxY = Math.max(maxY, p.y + NODE_H + PADDING);
    });

    svg.attr('viewBox', `0 0 ${maxX} ${maxY}`);

    // Background for zoom interaction
    svg.append('rect')
      .attr('width', maxX).attr('height', maxY)
      .attr('fill', 'transparent');

    // Zoom
    const zoom = d3.zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.2, 2.5])
      .filter((event) => {
        // Allow zoom on wheel, dblclick; pan on left-drag on empty space
        if (event.type === 'mousedown') return event.button === 0 && !event.ctrlKey;
        if (event.type === 'wheel') return true;
        if (event.type === 'dblclick') return true;
        return false;
      })
      .on('zoom', (event) => {
        mainGroupRef.current?.setAttribute('transform', event.transform.toString());
      });
    svg.call(zoom);
    zoomRef.current = zoom;

    const mainGroup = svg.append('g').attr('class', 'main-group');
    mainGroupRef.current = mainGroup.node();

    // Defs
    const defs = mainGroup.append('defs');

    defs.append('marker')
      .attr('id', 'dag-arrow')
      .attr('viewBox', '0 0 10 10')
      .attr('refX', 9).attr('refY', 5)
      .attr('markerWidth', 5).attr('markerHeight', 5)
      .attr('orient', 'auto')
      .append('path')
      .attr('d', 'M0,1 L8,5 L0,9 Q3,5 0,1 Z')
      .attr('fill', '#94A3B8');

    defs.append('filter').attr('id', 'node-shadow')
      .append('feDropShadow')
      .attr('dx', 0).attr('dy', 1).attr('stdDeviation', 2)
      .attr('flood-opacity', 0.06);

    // Edges
    const edgeGroup = mainGroup.append('g').attr('class', 'edges');

    edges.forEach((e) => {
      const from = pos[e.from];
      const to = pos[e.to];
      if (!from || !to) return;

      const x1 = from.x + NODE_W;
      const y1 = from.y + NODE_H / 2;
      const x2 = to.x;
      const y2 = to.y + NODE_H / 2;
      const pathStr = buildEdgePath(x1, y1, x2, y2);

      const g = edgeGroup.append('g')
        .attr('class', 'edge-group')
        .attr('cursor', 'pointer')
        .datum(e);

      g.append('path').attr('d', pathStr).attr('fill', 'none')
        .attr('stroke', 'transparent').attr('stroke-width', 14);

      g.append('path').attr('d', pathStr).attr('fill', 'none')
        .attr('stroke', '#94A3B8').attr('stroke-width', 1.8)
        .attr('marker-end', 'url(#dag-arrow)');

      g.on('mouseenter', function () {
        d3.select(this).select('path:nth-child(2)')
          .attr('stroke', '#EF4444').attr('stroke-width', 2.5);
      }).on('mouseleave', function () {
        d3.select(this).select('path:nth-child(2)')
          .attr('stroke', '#94A3B8').attr('stroke-width', 1.8);
      });

      if (!readonly && onEdgeDelete) {
        g.on('dblclick', () => onEdgeDelete(e.from, e.to));
      }
    });

    // Nodes
    const nodeG = mainGroup.selectAll<SVGGElement, DagNode>('g.dag-node')
      .data(nodes)
      .enter()
      .append('g')
      .attr('class', 'dag-node')
      .attr('transform', (d) => `translate(${pos[d.id]?.x ?? 0},${pos[d.id]?.y ?? 0})`)
      .attr('cursor', 'pointer')
      .on('click', (_, d) => onNodeClick(d.id))
      .on('dblclick', function (_, d) {
        const p = pos[d.id];
        if (!p || !svgRef.current) return;
        const svgEl = svgRef.current;
        const rect = svgEl.getBoundingClientRect();
        const tx = rect.width / 2 - p.x * 1.5;
        const ty = rect.height / 2 - p.y * 1.5;
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        (svg.transition().duration(350) as any).call(
          zoom.transform, d3.zoomIdentity.translate(tx, ty).scale(1.5),
        );
      });

    nodeG.append('rect')
      .attr('width', NODE_W).attr('height', NODE_H).attr('rx', 7)
      .attr('fill', '#fff')
      .attr('stroke', (d) => d.id === selectedNodeId ? '#7C3AED' : '#E2E8F0')
      .attr('stroke-width', (d) => d.id === selectedNodeId ? 2 : 1)
      .attr('filter', 'url(#node-shadow)');

    nodeG.append('rect')
      .attr('x', 0).attr('y', 0)
      .attr('width', 3.5).attr('height', NODE_H)
      .attr('fill', (d) => TYPE_COLORS[d.type || 'script'] || '#7C3AED');

    nodeG.append('text')
      .attr('x', 13).attr('y', 21)
      .attr('text-anchor', 'middle')
      .attr('font-size', 11)
      .attr('fill', (d) => TYPE_COLORS[d.type || 'script'] || '#7C3AED')
      .text((d) => TYPE_ICONS[d.type || 'script'] || '⬡');

    nodeG.append('text')
      .attr('x', 26).attr('y', 21)
      .attr('text-anchor', 'start')
      .attr('font-size', 11.5).attr('font-weight', 600)
      .attr('fill', '#1E293B')
      .text((d) => {
        const label = d.id || d.label || d.plugin || '';
        return label.length > 14 ? label.slice(0, 13) + '…' : label;
      });

    nodeG.filter((d) => d.status !== 'pending')
      .append('circle')
      .attr('cx', NODE_W - 5).attr('cy', 5).attr('r', 3.5)
      .attr('fill', (d) => STATUS_COLORS[d.status] || '#94A3B8');

    // Ports
    nodeG.append('g').attr('class', 'port-in')
      .append('circle')
      .attr('cx', 0).attr('cy', NODE_H / 2).attr('r', PORT_R)
      .attr('fill', '#fff').attr('stroke', '#CBD5E1').attr('stroke-width', 1.5);

    nodeG.append('g').attr('class', 'port-out')
      .append('circle')
      .attr('cx', NODE_W).attr('cy', NODE_H / 2).attr('r', PORT_R)
      .attr('fill', '#7C3AED').attr('stroke', '#fff').attr('stroke-width', 1.5);

    // Drag behaviors
    if (!readonly) {
      const nodeDrag = d3.drag<SVGGElement, DagNode>()
        .on('drag', function (event, d) {
          const cur = pos[d.id];
          if (!cur) return;
          cur.x += event.dx;
          cur.y += event.dy;
          d._x = cur.x;
          d._y = cur.y;
          d3.select(this).attr('transform', `translate(${cur.x},${cur.y})`);

          // Real-time edge update
          mainGroup.selectAll<SVGGElement, DagEdge>('g.edge-group').each(function (edgeData) {
            if (edgeData.from === d.id || edgeData.to === d.id) {
              const fp = pos[edgeData.from];
              const tp = pos[edgeData.to];
              if (fp && tp) {
                d3.select(this).selectAll('path').attr('d',
                  buildEdgePath(fp.x + NODE_W, fp.y + NODE_H / 2, tp.x, tp.y + NODE_H / 2));
              }
            }
          });
        });

      nodeG.call(nodeDrag as unknown as (sel: d3.Selection<SVGGElement, DagNode, SVGGElement, unknown>) => void);

      // Edge creation: mousedown on output port
      nodeG.select('g.port-out')
        .attr('cursor', 'crosshair')
        .on('mousedown', function (event: MouseEvent, d: DagNode) {
          event.stopPropagation();
          event.preventDefault();
          const p = pos[d.id];
          if (!p) return;
          dragRef.current = { from: d.id, sx: p.x + NODE_W, sy: p.y + NODE_H / 2 };
        });
    }

  }, [nodes, edges, selectedNodeId, onNodeClick, readonly, onEdgeDelete, computePositions, buildEdgePath]);

  // Global handlers for edge creation drag (direct D3 manipulation for preview line)
  useEffect(() => {
    const svg = svgRef.current;
    const mainGroup = mainGroupRef.current;
    if (!svg || !mainGroup) return;

    const toSVGCoords = (clientX: number, clientY: number): { x: number; y: number } => {
      if (!svg) return { x: 0, y: 0 };
      const rect = svg.getBoundingClientRect();
      const vb = svg.viewBox.baseVal;
      const scaleX = vb.width / rect.width;
      const scaleY = vb.height / rect.height;
      // We need to account for the zoom transform
      const t = svgRef.current ? (() => {
        const tm = mainGroupRef.current?.getAttribute('transform');
        if (!tm) return { x: 0, y: 0, k: 1 };
        const match = tm.match(/translate\(([^)]+)\)\s*scale\(([^)]+)\)/);
        if (!match) return { x: 0, y: 0, k: 1 };
        const [tx, ty] = match[1].split(',').map(Number);
        return { x: tx || 0, y: ty || 0, k: parseFloat(match[2]) || 1 };
      })() : { x: 0, y: 0, k: 1 };
      return {
        x: ((clientX - rect.left) * scaleX - t.x) / t.k,
        y: ((clientY - rect.top) * scaleY - t.y) / t.k,
      };
    };

    const handleMove = (e: MouseEvent) => {
      if (!dragRef.current || !mainGroup) return;
      const { x, y } = toSVGCoords(e.clientX, e.clientY);
      const d3Main = d3.select(mainGroup);

      // Ensure drag layer exists
      let dragLayer = d3Main.select<SVGGElement>('g.drag-layer');
      if (dragLayer.empty()) {
        dragLayer = d3Main.append('g').attr('class', 'drag-layer');
      }

      let preview = dragLayer.select<SVGPathElement>('path.drag-preview');
      if (preview.empty()) {
        preview = dragLayer.append('path')
          .attr('class', 'drag-preview')
          .attr('fill', 'none')
          .attr('stroke', '#7C3AED')
          .attr('stroke-width', 2)
          .attr('stroke-dasharray', '6 3')
          .attr('pointer-events', 'none');
        // Add arrow marker
        const defs = d3Main.select('defs');
        if (defs.select('#drag-arrow').empty()) {
          defs.append('marker')
            .attr('id', 'drag-arrow')
            .attr('viewBox', '0 0 10 10')
            .attr('refX', 9).attr('refY', 5)
            .attr('markerWidth', 5).attr('markerHeight', 5)
            .attr('orient', 'auto')
            .append('path')
            .attr('d', 'M0,1 L8,5 L0,9 Q3,5 0,1 Z')
            .attr('fill', '#7C3AED');
        }
        preview.attr('marker-end', 'url(#drag-arrow)');
      }
      preview.attr('d', buildEdgePath(dragRef.current.sx, dragRef.current.sy, x, y));
    };

    const handleUp = (e: MouseEvent) => {
      if (!dragRef.current) return;

      const { x, y } = toSVGCoords(e.clientX, e.clientY);
      const fromNodeId = dragRef.current.from;
      const pos = positionsRef.current;

      // Remove preview from drag layer
      if (mainGroup) {
        d3.select(mainGroup).select('g.drag-layer').remove();
      }

      // Check if dropped on an input port
      let hitTarget: string | null = null;
      const threshold = PORT_R + 6;
      for (const [nodeId, p] of Object.entries(pos)) {
        if (nodeId === fromNodeId) continue;
        const dist = Math.sqrt((x - p.x) ** 2 + (y - (p.y + NODE_H / 2)) ** 2);
        if (dist < threshold) {
          hitTarget = nodeId;
          break;
        }
      }

      if (hitTarget && onEdgeCreate) {
        onEdgeCreate(fromNodeId, hitTarget);
      } else if (!hitTarget && onAddNodeFromPort && plugins && plugins.length > 0) {
        setContextMenu({ fromNodeId, clientX: e.clientX, clientY: e.clientY });
      }

      dragRef.current = null;
    };

    window.addEventListener('mousemove', handleMove);
    window.addEventListener('mouseup', handleUp);
    return () => {
      window.removeEventListener('mousemove', handleMove);
      window.removeEventListener('mouseup', handleUp);
    };
  }, [onEdgeCreate, onAddNodeFromPort, plugins, buildEdgePath]);

  // Fit to view
  const fitToView = useCallback(() => {
    const svg = d3.select(svgRef.current);
    const pos = positionsRef.current;
    if (!pos || Object.keys(pos).length === 0) return;

    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    Object.values(pos).forEach((p) => {
      minX = Math.min(minX, p.x);
      minY = Math.min(minY, p.y);
      maxX = Math.max(maxX, p.x + NODE_W);
      maxY = Math.max(maxY, p.y + NODE_H);
    });

    const w = maxX - minX + PADDING * 2;
    const h = maxY - minY + PADDING * 2;
    const svgEl = svgRef.current;
    if (!svgEl) return;
    const rect = svgEl.getBoundingClientRect();
    const scale = Math.min(rect.width / w, rect.height / h, 1.1);
    const tx = rect.width / 2 - (minX + w / 2) * scale;
    const ty = rect.height / 2 - (minY + h / 2) * scale;

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (svg.transition().duration(400) as any).call(
      zoomRef.current.transform, d3.zoomIdentity.translate(tx, ty).scale(scale),
    );
  }, []);

  // Clear context menu on outside click
  useEffect(() => {
    if (!contextMenu) return;
    const handler = () => setContextMenu(null);
    window.addEventListener('click', handler);
    return () => window.removeEventListener('click', handler);
  }, [contextMenu]);

  if (nodes.length === 0) {
    return (
      <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', background: '#F8FAFC' }}>
        <div className="empty">
          <div className="empty-icon">🎨</div>
          <p>空白画布</p>
          <p style={{ fontSize: 12 }}>点击左侧插件添加步骤</p>
        </div>
      </div>
    );
  }

  return (
    <div ref={containerRef} style={{ flex: 1, overflow: 'hidden', background: '#F8FAFC', position: 'relative' }}>
      {/* Zoom controls */}
      <div style={{
        position: 'absolute', bottom: 16, right: 16, zIndex: 20,
        display: 'flex', flexDirection: 'column', gap: 4,
      }}>
        <button className="dag-zoom-btn" onClick={() => {
          const t = (() => {
            const tm = mainGroupRef.current?.getAttribute('transform');
            if (!tm) return { x: 0, y: 0, k: 1 };
            const m = tm.match(/translate\(([^)]+)\)\s*scale\(([^)]+)\)/);
            if (!m) return { x: 0, y: 0, k: 1 };
            return { x: parseFloat(m[1].split(',')[0]) || 0, y: parseFloat(m[1].split(',')[1]) || 0, k: parseFloat(m[2]) || 1 };
          })();
          const next = Math.min(t.k * 1.3, 2.5);
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          (d3.select(svgRef.current).transition().duration(200) as any).call(
            zoomRef.current.transform, d3.zoomIdentity.translate(t.x, t.y).scale(next),
          );
        }} title="放大">+</button>
        <button className="dag-zoom-btn" onClick={() => {
          const t = (() => {
            const tm = mainGroupRef.current?.getAttribute('transform');
            if (!tm) return { x: 0, y: 0, k: 1 };
            const m = tm.match(/translate\(([^)]+)\)\s*scale\(([^)]+)\)/);
            if (!m) return { x: 0, y: 0, k: 1 };
            return { x: parseFloat(m[1].split(',')[0]) || 0, y: parseFloat(m[1].split(',')[1]) || 0, k: parseFloat(m[2]) || 1 };
          })();
          const next = Math.max(t.k / 1.3, 0.2);
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          (d3.select(svgRef.current).transition().duration(200) as any).call(
            zoomRef.current.transform, d3.zoomIdentity.translate(t.x, t.y).scale(next),
          );
        }} title="缩小">−</button>
        <button className="dag-zoom-btn" onClick={fitToView} title="适配视图">⊡</button>
        {onAutoLayout && !readonly && (
          <button className="dag-zoom-btn" onClick={onAutoLayout} title="自动布局" style={{ fontSize: 13 }}>⟐</button>
        )}
      </div>

      <svg ref={svgRef} style={{
        display: 'block', width: '100%', height: '100%',
      }} />

      {/* Context menu for adding node when dropping on blank */}
      {contextMenu && plugins && onAddNodeFromPort && (
        <>
          <div style={{
            position: 'fixed', inset: 0, zIndex: 29,
          }} onClick={() => setContextMenu(null)} />
          <div className="dag-context-menu" style={{
            position: 'fixed',
            left: contextMenu.clientX,
            top: contextMenu.clientY,
            zIndex: 30,
            background: '#fff',
            borderRadius: 10,
            boxShadow: '0 8px 30px rgba(0,0,0,0.15)',
            border: '1px solid #E2E8F0',
            padding: '6px 0',
            minWidth: 190,
          }}>
            <div style={{
              padding: '6px 14px', fontSize: 10.5, color: '#94A3B8',
              fontWeight: 600, textTransform: 'uppercase', letterSpacing: 0.5,
            }}>
              添加步骤
            </div>
            {plugins.slice(0, 8).map((p) => (
              <div
                key={p.name + p.source}
                className="dag-context-item"
                onClick={() => {
                  onAddNodeFromPort(contextMenu.fromNodeId, p);
                  setContextMenu(null);
                }}
                style={{
                  padding: '8px 14px', cursor: 'pointer', fontSize: 13,
                  display: 'flex', alignItems: 'center', gap: 10,
                  transition: 'background 0.1s',
                }}
                onMouseEnter={(e) => { e.currentTarget.style.background = '#F1F5F9'; }}
                onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent'; }}
              >
                <span style={{
                  width: 28, height: 28, borderRadius: 6,
                  background: '#F1F5F9', display: 'flex',
                  alignItems: 'center', justifyContent: 'center',
                  fontSize: 13, color: '#7C3AED',
                }}>
                  {TYPE_ICONS[p.name] || '⬡'}
                </span>
                <div>
                  <div style={{ fontWeight: 600, fontSize: 12.5 }}>{p.label || p.name}</div>
                  <div style={{ fontSize: 10.5, color: '#94A3B8' }}>{p.runtime}</div>
                </div>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
