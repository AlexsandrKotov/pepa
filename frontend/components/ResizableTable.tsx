'use client';

import { useState, useRef, useCallback, useEffect, type ReactNode } from 'react';

export interface ColumnDef {
  key: string;
  label: string;
  width?: number; // default width in px
  minWidth?: number;
  render: (row: any, idx: number) => ReactNode;
}

interface ResizableTableProps {
  columns: ColumnDef[];
  data: any[];
  rowKey: (row: any, idx: number) => string;
  onColumnOrderChange?: (order: string[]) => void;
}

export default function ResizableTable({ columns, data, rowKey, onColumnOrderChange }: ResizableTableProps) {
  // Column order state
  const [colOrder, setColOrder] = useState<string[]>(() => columns.map(c => c.key));
  // Column widths state
  const [colWidths, setColWidths] = useState<Record<string, number>>(() => {
    const widths: Record<string, number> = {};
    columns.forEach(c => { widths[c.key] = c.width || 150; });
    return widths;
  });

  // Resizing state
  const [resizing, setResizing] = useState<{ key: string; startX: number; startWidth: number } | null>(null);
  // Dragging state (column reorder)
  const [dragging, setDragging] = useState<{ key: string; overKey: string | null } | null>(null);

  const tableRef = useRef<HTMLTableElement>(null);
  const headerRefs = useRef<Record<string, HTMLTableCellElement | null>>({});

  // Sync columns if they change externally
  useEffect(() => {
    setColOrder(prev => {
      const newKeys = columns.map(c => c.key);
      // Keep existing order for columns that still exist, add new ones at end
      const kept = prev.filter(k => newKeys.includes(k));
      const added = newKeys.filter(k => !prev.includes(k));
      return [...kept, ...added];
    });
  }, [columns]);

  // Get ordered columns
  const orderedColumns = colOrder
    .map(key => columns.find(c => c.key === key))
    .filter(Boolean) as ColumnDef[];

  // === RESIZE LOGIC ===
  const handleResizeStart = useCallback((key: string, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const startWidth = colWidths[key] || 150;
    setResizing({ key, startX: e.clientX, startWidth });
  }, [colWidths]);

  useEffect(() => {
    if (!resizing) return;

    const handleMouseMove = (e: MouseEvent) => {
      const diff = e.clientX - resizing.startX;
      const col = columns.find(c => c.key === resizing.key);
      const minW = col?.minWidth || 60;
      const newWidth = Math.max(minW, resizing.startWidth + diff);
      setColWidths(prev => ({ ...prev, [resizing.key]: newWidth }));
    };

    const handleMouseUp = () => {
      setResizing(null);
    };

    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
    return () => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };
  }, [resizing, columns]);

  // === DRAG REORDER LOGIC ===
  const handleDragStart = useCallback((key: string, e: React.DragEvent) => {
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', key);
    setDragging({ key, overKey: null });
  }, []);

  const handleDragOver = useCallback((key: string, e: React.DragEvent) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    if (dragging && dragging.key !== key) {
      setDragging(prev => prev ? { ...prev, overKey: key } : null);
    }
  }, [dragging]);

  const handleDrop = useCallback((targetKey: string, e: React.DragEvent) => {
    e.preventDefault();
    if (!dragging || dragging.key === targetKey) {
      setDragging(null);
      return;
    }

    const newOrder = [...colOrder];
    const fromIdx = newOrder.indexOf(dragging.key);
    const toIdx = newOrder.indexOf(targetKey);
    if (fromIdx >= 0 && toIdx >= 0) {
      newOrder.splice(fromIdx, 1);
      newOrder.splice(toIdx, 0, dragging.key);
      setColOrder(newOrder);
      onColumnOrderChange?.(newOrder);
    }
    setDragging(null);
  }, [dragging, colOrder, onColumnOrderChange]);

  const handleDragEnd = useCallback(() => {
    setDragging(null);
  }, []);

  // Total width
  const totalWidth = orderedColumns.reduce((sum, col) => sum + (colWidths[col.key] || 150), 0);

  return (
    <div className="card overflow-x-auto">
      <div className="table-container">
        <table
          ref={tableRef}
          style={{ width: totalWidth, tableLayout: 'fixed' }}
          className="border-collapse"
        >
          <thead>
            <tr>
              {orderedColumns.map((col, colIdx) => {
                const width = colWidths[col.key] || 150;
                const isDragging = dragging?.key === col.key;
                const isOver = dragging?.overKey === col.key;
                const isLeftOfDrag = isOver && dragging && colOrder.indexOf(col.key) > colOrder.indexOf(dragging.key);
                const isRightOfDrag = isOver && dragging && colOrder.indexOf(col.key) < colOrder.indexOf(dragging.key);

                return (
                  <th
                    key={col.key}
                    ref={el => { headerRefs.current[col.key] = el; }}
                    draggable
                    onDragStart={e => handleDragStart(col.key, e)}
                    onDragOver={e => handleDragOver(col.key, e)}
                    onDrop={e => handleDrop(col.key, e)}
                    onDragEnd={handleDragEnd}
                    style={{
                      width,
                      minWidth: col.minWidth || 60,
                      cursor: 'grab',
                      position: 'relative',
                      opacity: isDragging ? 0.4 : 1,
                      borderLeft: isLeftOfDrag ? '2px solid var(--accent)' : undefined,
                      borderRight: isRightOfDrag ? '2px solid var(--accent)' : undefined,
                      userSelect: 'none',
                      padding: '8px 12px',
                      fontSize: '11px',
                      fontWeight: 600,
                      textTransform: 'uppercase',
                      letterSpacing: '0.05em',
                      color: 'var(--text-tertiary)',
                      borderBottom: '1px solid var(--border)',
                      background: isDragging ? 'var(--border-light)' : undefined,
                      whiteSpace: 'nowrap',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                    }}
                  >
                    <span className="flex items-center gap-1">
                      <span className="text-[10px] opacity-40 cursor-grab" title="Drag to reorder">⠿</span>
                      {col.label}
                    </span>
                    {/* Resize handle */}
                    <div
                      onMouseDown={e => handleResizeStart(col.key, e)}
                      onClick={e => e.stopPropagation()}
                      style={{
                        position: 'absolute',
                        right: 0,
                        top: 0,
                        bottom: 0,
                        width: '6px',
                        cursor: 'col-resize',
                        zIndex: 10,
                      }}
                      onMouseEnter={e => {
                        (e.currentTarget as HTMLElement).style.background = 'var(--accent)';
                        (e.currentTarget as HTMLElement).style.opacity = '0.3';
                      }}
                      onMouseLeave={e => {
                        (e.currentTarget as HTMLElement).style.background = '';
                        (e.currentTarget as HTMLElement).style.opacity = '';
                      }}
                    />
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody>
            {data.map((row, idx) => (
              <tr key={rowKey(row, idx)}>
                {orderedColumns.map(col => (
                  <td
                    key={col.key}
                    style={{
                      width: colWidths[col.key] || 150,
                      padding: '6px 12px',
                      fontSize: '12px',
                      borderBottom: '1px solid var(--border-light)',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {col.render(row, idx)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
