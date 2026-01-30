const { createApp, ref, watch, onMounted, nextTick } = Vue;

const TreeNode = {
  name: 'TreeNode',
  props: ['node', 'depth', 'index', 'parentChildren', 'dragEnabled'],
  emits: ['toggle', 'select', 'reorder', 'move-into'],
  template: `
    <div class="tree-node">
      <div
        class="tree-row"
        :class="{ selected: node.selected, dragging: isDragging, 'drag-over': isDragOver, 'drag-over-bottom': isDragOverBottom, 'drag-over-into': isDragOverInto }"
        :style="{ paddingLeft: depth * 20 + 12 + 'px' }"
        :draggable="dragEnabled && parentChildren !== null"
        @click="handleClick"
        @dragstart="onDragStart"
        @dragend="onDragEnd"
        @dragover="onDragOver"
        @dragleave="onDragLeave"
        @drop="onDrop"
      >
        <span v-if="dragEnabled && parentChildren !== null" class="drag-handle" @mousedown.stop>⋮⋮</span>
        <span class="tree-toggle" @click.stop="$emit('toggle', node)">
          <template v-if="node.children && node.children.length">
            {{ node.expanded ? '▼' : '▶' }}
          </template>
        </span>

        <input
          v-if="node.isLeaf"
          type="checkbox"
          class="tree-checkbox"
          :checked="node.selected"
          @click.stop="$emit('select', node)"
        >
        <span v-else class="tree-icon" :class="node.isArray ? 'arr' : 'obj'">
          {{ node.isArray ? '[ ]' : '{ }' }}
        </span>

        <span class="tree-key">{{ node.key }}</span>

        <span v-if="!node.isLeaf" class="tree-type">
          {{ node.isArray ? node.arrayLength + ' items' : (node.children ? node.children.length : 0) + ' keys' }}
        </span>
      </div>

      <template v-if="node.expanded && node.children">
        <tree-node
          v-for="(child, idx) in node.children"
          :key="child.key + '-' + idx"
          :node="child"
          :depth="depth + 1"
          :index="idx"
          :parent-children="node.children"
          :drag-enabled="dragEnabled"
          @toggle="$emit('toggle', $event)"
          @select="$emit('select', $event)"
          @reorder="$emit('reorder', $event)"
          @move-into="$emit('move-into', $event)"
        />
      </template>
    </div>
  `,
  data() {
    return {
      isDragging: false,
      isDragOver: false,
      isDragOverBottom: false,
      isDragOverInto: false
    };
  },
  methods: {
    handleClick() {
      if (this.node.isLeaf) {
        this.$emit('select', this.node);
      } else {
        this.$emit('toggle', this.node);
      }
    },
    onDragStart(e) {
      if (!this.dragEnabled || this.parentChildren === null) {
        e.preventDefault();
        return;
      }
      this.isDragging = true;
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('application/json', JSON.stringify({
        path: this.node.path,
        key: this.node.key,
        index: this.index
      }));
    },
    onDragEnd() {
      this.isDragging = false;
    },
    onDragOver(e) {
      if (!this.dragEnabled) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';

      const rect = e.currentTarget.getBoundingClientRect();
      const relY = e.clientY - rect.top;
      const height = rect.height;

      // For container nodes (array/object), check if dropping INTO the center
      const isContainer = !this.node.isLeaf && (this.node.isArray || this.node.children);

      if (isContainer && this.parentChildren !== null) {
        // Divide into 3 zones: top 25% = before, middle 50% = into, bottom 25% = after
        if (relY < height * 0.25) {
          this.isDragOver = true;
          this.isDragOverBottom = false;
          this.isDragOverInto = false;
        } else if (relY > height * 0.75) {
          this.isDragOver = false;
          this.isDragOverBottom = true;
          this.isDragOverInto = false;
        } else {
          this.isDragOver = false;
          this.isDragOverBottom = false;
          this.isDragOverInto = true;
        }
      } else if (isContainer && this.parentChildren === null) {
        // Root-level container can only accept drops INTO
        this.isDragOver = false;
        this.isDragOverBottom = false;
        this.isDragOverInto = true;
      } else if (this.parentChildren !== null) {
        // Leaf nodes: only reorder (top/bottom)
        const midY = rect.top + height / 2;
        if (e.clientY < midY) {
          this.isDragOver = true;
          this.isDragOverBottom = false;
        } else {
          this.isDragOver = false;
          this.isDragOverBottom = true;
        }
        this.isDragOverInto = false;
      }
    },
    onDragLeave(e) {
      // Only clear if leaving the element entirely
      const rect = e.currentTarget.getBoundingClientRect();
      if (e.clientX < rect.left || e.clientX > rect.right ||
          e.clientY < rect.top || e.clientY > rect.bottom) {
        this.isDragOver = false;
        this.isDragOverBottom = false;
        this.isDragOverInto = false;
      }
    },
    onDrop(e) {
      e.preventDefault();
      e.stopPropagation();

      const wasDragOverInto = this.isDragOverInto;
      this.isDragOver = false;
      this.isDragOverBottom = false;
      this.isDragOverInto = false;

      if (!this.dragEnabled) return;

      try {
        const dataStr = e.dataTransfer.getData('application/json');
        if (!dataStr) return;

        const data = JSON.parse(dataStr);
        const fromPath = data.path;
        const fromKey = data.key;
        const toPath = this.node.path;

        if (fromPath === toPath) return;

        // Prevent dropping a parent into its own child
        if (toPath.startsWith(fromPath + '.') || toPath.startsWith(fromPath + '[')) return;

        // If dropping INTO a container
        if (wasDragOverInto && !this.node.isLeaf) {
          this.$emit('move-into', {
            fromPath,
            fromKey,
            targetNode: this.node
          });
          return;
        }

        // Otherwise, reorder within same parent
        if (this.parentChildren === null) return;

        const rect = e.currentTarget.getBoundingClientRect();
        const midY = rect.top + rect.height / 2;
        const insertAfter = e.clientY >= midY;

        this.$emit('reorder', {
          fromPath,
          toPath,
          insertAfter,
          parentChildren: this.parentChildren
        });
      } catch (err) {
        console.error('Drop error:', err);
      }
    }
  }
};

function initApp(initialData, dirMode, currentFileName) {
  createApp({
    components: { TreeNode },
    setup() {
      const rawData = ref(initialData);
      const tree = ref(null);
      const result = ref('');
      const error = ref('');
      const format = ref('json');
      const expression = ref('.');
      const dragEnabled = ref(false);
      const compressPath = ref(true);  // Path compression enabled by default
      // Track moved nodes: { fromPath: string, toPath: string, key: string }[]
      const movedNodes = ref([]);

      // Directory mode
      const dirModeRef = ref(dirMode);
      const fileList = ref([]);
      const currentFile = ref(currentFileName);

      function buildTree(data, key = 'root', path = '.', isArrayItem = false, originalIndex = 0) {
        const node = {
          key,
          path,
          value: data,
          expanded: path === '.',
          selected: false,
          isLeaf: false,
          isArray: false,
          isArrayItem,
          arrayLength: 0,
          children: null,
          displayValue: '',
          valueType: '',
          originalIndex
        };

        if (Array.isArray(data)) {
          node.isArray = true;
          node.arrayLength = data.length;
          // Directly show fields from first element (no [*] wrapper)
          if (data.length > 0 && typeof data[0] === 'object') {
            const templatePath = path + '[]';
            node.children = Object.entries(data[0]).map(([k, v], idx) =>
              buildTree(v, k, `${templatePath}.${k}`, true, idx)
            );
          }
        } else if (data && typeof data === 'object') {
          node.children = Object.entries(data).map(([k, v], idx) =>
            buildTree(v, k, path === '.' ? `.${k}` : `${path}.${k}`, isArrayItem, idx)
          );
        } else {
          node.isLeaf = true;
          node.displayValue = formatValue(data);
          node.valueType = getValueType(data);
        }

        return node;
      }

      function formatValue(v) {
        if (v === null) return 'null';
        if (typeof v === 'string') return v.length > 30 ? `"${v.slice(0, 30)}…"` : `"${v}"`;
        if (typeof v === 'boolean') return v ? 'true' : 'false';
        return String(v);
      }

      function getValueType(v) {
        if (v === null) return 'null';
        if (typeof v === 'string') return 'str';
        if (typeof v === 'number') return 'num';
        if (typeof v === 'boolean') return 'bool';
        return '';
      }

      function toggleNode(node) {
        node.expanded = !node.expanded;
      }

      function selectNode(node) {
        node.selected = !node.selected;
        updateExpression();
      }

      function handleReorder({ fromPath, toPath, insertAfter, parentChildren }) {
        const fromIdx = parentChildren.findIndex(c => c.path === fromPath);
        const toIdx = parentChildren.findIndex(c => c.path === toPath);

        if (fromIdx === -1 || toIdx === -1 || fromIdx === toIdx) return;

        // Calculate new position
        let newIdx = toIdx;
        if (fromIdx < toIdx) newIdx--;
        if (insertAfter) newIdx++;

        // Perform the move
        const [moved] = parentChildren.splice(fromIdx, 1);
        parentChildren.splice(newIdx, 0, moved);

        // Force Vue to detect the change
        tree.value = { ...tree.value };

        // Immediately update and run query
        const selected = collectSelected(tree.value);
        if (selected.length > 0) {
          expression.value = buildNestedExpr(selected);
        }
        runQuery();
      }

      // Find a node by path in the tree
      function findNodeByPath(node, path) {
        if (node.path === path) return node;
        if (node.children) {
          for (const child of node.children) {
            const found = findNodeByPath(child, path);
            if (found) return found;
          }
        }
        return null;
      }

      // Find the parent node and index of a node by path
      function findParentOfNode(root, targetPath, parent = null, index = -1) {
        if (root.path === targetPath) {
          return { parent, index };
        }
        if (root.children) {
          for (let i = 0; i < root.children.length; i++) {
            const result = findParentOfNode(root.children[i], targetPath, root, i);
            if (result.parent !== null) return result;
          }
        }
        return { parent: null, index: -1 };
      }

      // Update paths recursively when a node is moved
      function updateNodePaths(node, newBasePath) {
        const isArrayContext = newBasePath.includes('[]');
        node.path = newBasePath;
        node.isArrayItem = isArrayContext;

        if (node.children) {
          node.children.forEach(child => {
            const childPath = node.isArray
              ? `${newBasePath}[].${child.key}`
              : `${newBasePath}.${child.key}`;
            updateNodePaths(child, childPath);
          });
        }
      }

      // Handle moving a node INTO a container
      function handleMoveInto({ fromPath, fromKey, targetNode }) {
        // Find source node and its parent
        const sourceNode = findNodeByPath(tree.value, fromPath);
        if (!sourceNode) return;

        const { parent: sourceParent, index: sourceIndex } = findParentOfNode(tree.value, fromPath);
        if (!sourceParent || sourceIndex === -1) return;

        // Don't allow moving into self or own children
        if (targetNode.path === fromPath || targetNode.path.startsWith(fromPath + '.')) return;

        // Remove from source parent
        const [movedNode] = sourceParent.children.splice(sourceIndex, 1);

        // Calculate new path based on target
        let newPath;
        if (targetNode.isArray) {
          // Moving into an array: add as a field to array items
          newPath = `${targetNode.path}[].${movedNode.key}`;
        } else {
          // Moving into an object
          newPath = `${targetNode.path}.${movedNode.key}`;
        }

        // Update the moved node's paths recursively
        updateNodePaths(movedNode, newPath);

        // Record the move for expression building
        movedNodes.value.push({
          fromPath: fromPath,
          toPath: targetNode.path,
          key: movedNode.key,
          originalPath: fromPath  // Keep original for jq source
        });

        // Add to target's children
        if (!targetNode.children) {
          targetNode.children = [];
        }
        targetNode.children.push(movedNode);

        // Expand target to show the moved node
        targetNode.expanded = true;

        // Force Vue to detect the change
        tree.value = { ...tree.value };

        // Update expression if there are selections
        const selected = collectSelected(tree.value);
        if (selected.length > 0) {
          expression.value = buildNestedExpr(selected);
        }
        runQuery();
      }

      function collectSelected(node, paths = [], orderCounter = { value: 0 }) {
        if (node.selected && node.isLeaf) {
          // Check if this node is under a moved parent
          let sourcePath = node.path;
          let isMoved = false;

          for (const move of movedNodes.value) {
            // Check if the node's path starts with the moved node's new path
            const movedNewPathPrefix = move.toPath.endsWith('[]')
              ? `${move.toPath.slice(0, -2)}[].${move.key}`
              : `${move.toPath}.${move.key}`;

            if (node.path.startsWith(movedNewPathPrefix)) {
              // This node is under a moved parent, calculate original source path
              const suffix = node.path.slice(movedNewPathPrefix.length);
              sourcePath = move.originalPath + suffix;
              isMoved = true;
              break;
            }
          }

          paths.push({
            path: node.path,  // New display path (for output structure)
            sourcePath: sourcePath,  // Original data path (for getting data)
            key: node.key,
            originalIndex: node.originalIndex,
            order: orderCounter.value++,
            isMoved: isMoved
          });
        }
        if (node.children) {
          node.children.forEach(c => collectSelected(c, paths, orderCounter));
        }
        return paths;
      }

      // Build nested structure expression that preserves hierarchy
      // Paths are already in tree order (after drag reordering)
      function buildNestedExpr(paths) {
        // Check if we have any moved nodes - need $root binding
        const hasMoved = paths.some(p => p.isMoved);

        // Parse each path into segments, keep original order
        const parsed = paths.map((p) => {
          // Parse display path for output structure
          const displaySegments = [];
          let current = p.path.slice(1); // remove leading dot

          const regex = /([^.\[\]]+)(\[\])?\.?/g;
          let match;
          while ((match = regex.exec(current)) !== null) {
            if (match[2]) {
              displaySegments.push(match[1] + '[]');
            } else {
              displaySegments.push(match[1]);
            }
          }

          // Parse source path for data extraction
          const sourceSegments = [];
          let sourceCurrent = p.sourcePath.slice(1);
          regex.lastIndex = 0;
          while ((match = regex.exec(sourceCurrent)) !== null) {
            if (match[2]) {
              sourceSegments.push(match[1] + '[]');
            } else {
              sourceSegments.push(match[1]);
            }
          }

          return {
            displaySegments,  // For output structure
            sourceSegments,   // For data extraction
            key: p.key,
            path: p.path,
            sourcePath: p.sourcePath,
            order: p.order,
            isMoved: p.isMoved
          };
        });

        // Separate array paths from non-array paths (based on display structure)
        const arrayPaths = parsed.filter(p => p.displaySegments.some(s => s.endsWith('[]')));
        const nonArrayPaths = parsed.filter(p => !p.displaySegments.some(s => s.endsWith('[]')));

        // Build tree structure with order info
        function buildTreeWithOrder(paths) {
          if (paths.length === 0) return null;

          const tree = new Map();
          paths.forEach(({ displaySegments, sourcePath, order, isMoved }) => {
            let node = tree;
            displaySegments.forEach((seg, i) => {
              if (i === displaySegments.length - 1) {
                node.set(seg, { leaf: true, sourcePath: sourcePath, order: order, isMoved: isMoved });
              } else {
                if (!node.has(seg) || node.get(seg).leaf) {
                  node.set(seg, { children: new Map(), order: order });
                }
                node = node.get(seg).children;
              }
            });
          });
          return tree;
        }

        // Convert tree to jq expression with optional path compression
        // e.g., pk -> season -> tier -> tier_name becomes "pk.season.tier.tier_name": ...
        function treeToExpr(tree, useRoot = false) {
          if (!tree || tree.size === 0) return '';

          const entries = Array.from(tree.entries());
          // Sort by order to preserve drag reorder
          entries.sort((a, b) => {
            const orderA = a[1].order ?? 0;
            const orderB = b[1].order ?? 0;
            return orderA - orderB;
          });

          const parts = entries.map(([k, v]) => {
            if (v.leaf) {
              // Leaf node: extract value from source path
              const prefix = (v.isMoved && useRoot) ? '$root' : '';
              return `${k}: ${prefix}${v.sourcePath}`;
            } else if (compressPath.value) {
              // Nested object with compression - check if we can compress the path
              let pathParts = [k];
              let current = v.children;

              while (current && current.size === 1) {
                const [childKey, childVal] = current.entries().next().value;
                if (childVal.leaf) {
                  // Reached a leaf - compress the path
                  pathParts.push(childKey);
                  const compressedKey = pathParts.join('.');
                  const prefix = (childVal.isMoved && useRoot) ? '$root' : '';
                  return `"${compressedKey}": ${prefix}${childVal.sourcePath}`;
                } else if (childVal.children && childVal.children.size === 1) {
                  // Continue compressing
                  pathParts.push(childKey);
                  current = childVal.children;
                } else {
                  // Multiple children or end of chain - stop compressing here
                  pathParts.push(childKey);
                  const inner = treeToExpr(childVal.children, useRoot);
                  const compressedKey = pathParts.join('.');
                  return `"${compressedKey}": ${inner}`;
                }
              }

              // No compression possible (multiple children at this level)
              const inner = treeToExpr(v.children, useRoot);
              return `${k}: ${inner}`;
            } else {
              // No compression - full nested structure
              const inner = treeToExpr(v.children, useRoot);
              return `${k}: ${inner}`;
            }
          });

          return `{${parts.join(', ')}}`;
        }

        // Handle array paths - group by array base, preserve order
        // Build nested tree structure for fields within array
        function buildArrayExpr(paths, useRoot = false) {
          const groups = new Map();
          paths.forEach(({ displaySegments, sourceSegments, sourcePath, order, isMoved }) => {
            const arrayIdx = displaySegments.findIndex(s => s.endsWith('[]'));
            const baseParts = displaySegments.slice(0, arrayIdx + 1);
            const base = '.' + baseParts.map(s => s.replace('[]', '')).join('.') + '[]';
            const rest = displaySegments.slice(arrayIdx + 1);

            if (!groups.has(base)) groups.set(base, []);
            groups.get(base).push({ rest, sourcePath, order, isMoved });
          });

          const exprs = [];
          groups.forEach((items, base) => {
            // Sort by order first
            items.sort((a, b) => a.order - b.order);

            // Build a tree structure for nested fields
            const fieldTree = new Map();
            items.forEach(({ rest, sourcePath, isMoved, order }) => {
              if (rest.length === 0) return;

              let node = fieldTree;
              rest.forEach((seg, i) => {
                if (i === rest.length - 1) {
                  // Leaf node
                  node.set(seg, { leaf: true, sourcePath, isMoved, order });
                } else {
                  // Intermediate node
                  if (!node.has(seg)) {
                    node.set(seg, { children: new Map(), order });
                  }
                  const existing = node.get(seg);
                  if (existing.children) {
                    node = existing.children;
                  }
                }
              });
            });

            // Convert field tree to jq expression with optional path compression
            // e.g., pk -> season -> tier -> tier_name becomes "pk.season.tier": {tier_name: ...}
            function fieldTreeToExpr(tree, currentArrayBase) {
              const entries = Array.from(tree.entries());
              entries.sort((a, b) => (a[1].order ?? 0) - (b[1].order ?? 0));

              const parts = entries.map(([key, val]) => {
                if (val.leaf) {
                  if (val.isMoved) {
                    // Moved field: use $root to get from original path
                    return `${key}: ${useRoot ? '$root' : ''}${val.sourcePath}`;
                  } else {
                    // Normal field: extract relative path from sourcePath
                    let relPath = val.sourcePath;
                    if (currentArrayBase && relPath.startsWith(currentArrayBase)) {
                      relPath = relPath.slice(currentArrayBase.length);
                      if (!relPath.startsWith('.')) relPath = '.' + relPath;
                    }
                    return `${key}: ${relPath}`;
                  }
                } else if (compressPath.value) {
                  // Nested object with compression - check if we can compress the path
                  let pathParts = [key];
                  let current = val.children;

                  while (current && current.size === 1) {
                    const [childKey, childVal] = current.entries().next().value;
                    if (childVal.leaf) {
                      // Reached a leaf - compress the path
                      pathParts.push(childKey);
                      const compressedKey = pathParts.join('.');
                      if (childVal.isMoved) {
                        return `"${compressedKey}": ${useRoot ? '$root' : ''}${childVal.sourcePath}`;
                      } else {
                        let relPath = childVal.sourcePath;
                        if (currentArrayBase && relPath.startsWith(currentArrayBase)) {
                          relPath = relPath.slice(currentArrayBase.length);
                          if (!relPath.startsWith('.')) relPath = '.' + relPath;
                        }
                        return `"${compressedKey}": ${relPath}`;
                      }
                    } else if (childVal.children && childVal.children.size === 1) {
                      // Continue compressing
                      pathParts.push(childKey);
                      current = childVal.children;
                    } else {
                      // Multiple children or end of chain - stop compressing here
                      pathParts.push(childKey);
                      const inner = fieldTreeToExpr(childVal.children, currentArrayBase);
                      const compressedKey = pathParts.join('.');
                      return `"${compressedKey}": ${inner}`;
                    }
                  }

                  // No compression possible (multiple children at this level)
                  const inner = fieldTreeToExpr(val.children, currentArrayBase);
                  return `${key}: ${inner}`;
                } else {
                  // No compression - full nested structure
                  const inner = fieldTreeToExpr(val.children, currentArrayBase);
                  return `${key}: ${inner}`;
                }
              });

              return `{${parts.join(', ')}}`;
            }

            // Get parent path (before [])
            const arrayIdx = base.lastIndexOf('[]');
            const parentPath = base.slice(0, arrayIdx);
            const parentSegments = parentPath.slice(1).split('.').filter(s => s);

            let innerExpr = fieldTreeToExpr(fieldTree, base);

            if (parentSegments.length === 0) {
              exprs.push(`[.[] | ${innerExpr}]`);
            } else {
              // Build with parent structure
              let result = `[${base} | ${innerExpr}]`;
              for (let i = parentSegments.length - 1; i >= 0; i--) {
                result = `{${parentSegments[i]}: ${result}}`;
              }
              exprs.push(result);
            }
          });

          return exprs;
        }

        const results = [];

        if (nonArrayPaths.length > 0) {
          const tree = buildTreeWithOrder(nonArrayPaths);
          results.push(treeToExpr(tree, hasMoved));
        }

        if (arrayPaths.length > 0) {
          results.push(...buildArrayExpr(arrayPaths, hasMoved));
        }

        // Merge results
        let finalExpr;
        if (results.length === 1) {
          finalExpr = results[0];
        } else if (results.length > 1) {
          // Merge using * operator for objects
          finalExpr = results.join(' * ');
        } else {
          finalExpr = '.';
        }

        // Wrap with $root binding if needed
        if (hasMoved && finalExpr !== '.') {
          finalExpr = `. as $root | ${finalExpr}`;
        }

        return finalExpr;
      }

      function updateExpression() {
        const selected = collectSelected(tree.value);
        if (selected.length === 0) {
          expression.value = '.';
          runQuery();
          return;
        }

        expression.value = buildNestedExpr(selected);
        runQuery();
      }

      function expandAll() {
        function expand(node) {
          if (node.children) {
            node.expanded = true;
            node.children.forEach(expand);
          }
        }
        expand(tree.value);
      }

      function collapseAll() {
        function collapse(node) {
          if (node.children) {
            node.expanded = node.path === '.';
            node.children.forEach(collapse);
          }
        }
        collapse(tree.value);
      }

      // Collapse nodes that have no selected leaves in their subtree
      function collapseEmpty() {
        function hasSelectedLeaf(node) {
          if (node.isLeaf) return node.selected;
          if (!node.children) return false;
          return node.children.some(hasSelectedLeaf);
        }

        function process(node) {
          if (!node.children) return;

          // Check if this node has any selected leaves
          const hasSelection = hasSelectedLeaf(node);

          if (hasSelection) {
            // Keep expanded if has selections, but collapse children without selections
            node.expanded = true;
            node.children.forEach(child => {
              if (child.children) {
                if (hasSelectedLeaf(child)) {
                  process(child);
                } else {
                  child.expanded = false;
                }
              }
            });
          } else {
            // No selections: collapse (except root)
            node.expanded = node.path === '.';
          }
        }

        process(tree.value);
      }

      async function runQuery() {
        try {
          const res = await fetch('/api/query', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              data: rawData.value,
              expression: expression.value,
              format: format.value
            })
          });
          const data = await res.json();
          if (data.error) {
            error.value = data.error;
            result.value = '';
          } else {
            result.value = data.result;
            error.value = '';
          }
        } catch (e) {
          error.value = e.message;
        }
      }

      function copyResult() {
        navigator.clipboard.writeText(result.value);
      }

      // Load file list from server
      async function fetchFileList() {
        if (!dirModeRef.value) return;
        try {
          const res = await fetch('/api/files');
          const data = await res.json();
          if (data.files) {
            fileList.value = data.files;
          }
        } catch (e) {
          console.error('Failed to fetch file list:', e);
        }
      }

      // Load a specific file
      async function loadFile(filename) {
        if (filename === currentFile.value) return;
        try {
          const res = await fetch('/api/load', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ filename })
          });
          const data = await res.json();
          if (data.error) {
            error.value = data.error;
            return;
          }
          // Update state
          currentFile.value = filename;
          rawData.value = data.data;
          movedNodes.value = [];  // Reset moved nodes tracking
          tree.value = buildTree(rawData.value);
          expression.value = '.';
          runQuery();
        } catch (e) {
          error.value = e.message;
        }
      }

      // Refresh file list (for newly added files)
      async function refreshFileList() {
        await fetchFileList();
      }

      onMounted(() => {
        tree.value = buildTree(rawData.value);
        runQuery();
        fetchFileList();

        // Hot reload: connect to SSE and reload on server restart
        let lastServerId = null;
        function connectReload() {
          const es = new EventSource('/api/reload');
          es.onmessage = (e) => {
            const serverId = e.data;
            if (lastServerId !== null && lastServerId !== serverId) {
              // Server restarted, reload page
              window.location.reload();
            }
            lastServerId = serverId;
          };
          es.onerror = () => {
            es.close();
            // Reconnect after delay
            setTimeout(connectReload, 1000);
          };
        }
        connectReload();
      });

      watch(format, runQuery);
      watch(compressPath, updateExpression);

      return {
        tree, result, error, format, expression, dragEnabled, compressPath,
        dirMode: dirModeRef, fileList, currentFile,
        toggleNode, selectNode, handleReorder, handleMoveInto, expandAll, collapseAll, collapseEmpty,
        runQuery, copyResult, loadFile, refreshFileList
      };
    }
  }).mount('#app');
}
