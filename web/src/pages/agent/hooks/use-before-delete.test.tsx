import type { RAGFlowNodeType } from '@/interfaces/database/agent';
import { act, renderHook } from '@testing-library/react';
import { Edge } from '@xyflow/react';
import { NodeHandleId, Operator } from '../constant';
import useGraphStore from '../store';
import { useBeforeDelete } from './use-before-delete';

const createNode = (
  id: string,
  label: Operator,
  options: Partial<RAGFlowNodeType> = {},
): RAGFlowNodeType => ({
  id,
  type: 'ragNode',
  position: { x: 0, y: 0 },
  data: {
    label,
    name: id,
    form: {},
  },
  ...options,
});

const createEdge = (
  id: string,
  source: string,
  target: string,
  options: Partial<Edge> = {},
): Edge => ({
  id,
  source,
  target,
  ...options,
});

async function deleteFromHook(
  handle: ReturnType<typeof useBeforeDelete>['handleBeforeDelete'],
  input: Parameters<typeof handle>[0],
) {
  const result = await handle(input);
  if (typeof result === 'boolean') {
    throw new Error('Expected explicit nodes and edges from the delete hook');
  }
  return result;
}

describe('useBeforeDelete', () => {
  beforeEach(() => {
    useGraphStore.setState({
      nodes: [],
      edges: [],
      selectedNodeIds: [],
      selectedEdgeIds: [],
      clickedNodeId: '',
      clickedToolId: '',
    });
  });

  it('expands iteration deletion to descendants and all touching edges', async () => {
    const nodes = [
      createNode('iteration:0', Operator.Iteration, { type: 'group' }),
      createNode('iterationStart:0', Operator.IterationStart, {
        parentId: 'iteration:0',
        type: 'iterationStartNode',
      }),
      createNode('message:0', Operator.Message, { parentId: 'iteration:0' }),
      createNode('message:1', Operator.Message, { parentId: 'message:0' }),
      createNode('message:out', Operator.Message),
    ];

    const edges = [
      createEdge('e1', 'iterationStart:0', 'message:0'),
      createEdge('e2', 'message:0', 'message:1'),
      createEdge('e3', 'message:0', 'message:out'),
      createEdge('e4', 'message:out', 'message:1'),
    ];

    useGraphStore.setState({ nodes, edges });

    const { result } = renderHook(() => useBeforeDelete());
    let deletion: Awaited<ReturnType<typeof deleteFromHook>> | undefined;
    await act(async () => {
      deletion = await deleteFromHook(result.current.handleBeforeDelete, {
        nodes: [nodes[0]],
        edges: [],
      });
    });

    expect(deletion?.nodes.map((node) => node.id).sort()).toEqual(
      ['iteration:0', 'iterationStart:0', 'message:0', 'message:1'].sort(),
    );
    expect(deletion?.edges.map((edge) => edge.id).sort()).toEqual(
      ['e1', 'e2', 'e3', 'e4'].sort(),
    );
  });

  it('keeps begin and detached iteration-start protected', async () => {
    const beginNode = createNode('begin', Operator.Begin);
    const iterationNode = createNode('iteration:0', Operator.Iteration, {
      type: 'group',
    });
    const iterationStartNode = createNode(
      'iterationStart:0',
      Operator.IterationStart,
      {
        parentId: 'iteration:0',
        type: 'iterationStartNode',
      },
    );

    useGraphStore.setState({
      nodes: [beginNode, iterationNode, iterationStartNode],
      edges: [],
    });

    const { result } = renderHook(() => useBeforeDelete());
    let beginDeletion: Awaited<ReturnType<typeof deleteFromHook>> | undefined;
    let startDeletion: Awaited<ReturnType<typeof deleteFromHook>> | undefined;
    await act(async () => {
      beginDeletion = await deleteFromHook(result.current.handleBeforeDelete, {
        nodes: [beginNode],
        edges: [],
      });
      startDeletion = await deleteFromHook(result.current.handleBeforeDelete, {
        nodes: [iterationStartNode],
        edges: [],
      });
    });

    expect(beginDeletion?.nodes).toEqual([]);
    expect(startDeletion?.nodes).toEqual([]);
  });

  it('allows deleting a selected edge inside iteration without deleting nodes', async () => {
    const nodes = [
      createNode('iteration:0', Operator.Iteration, { type: 'group' }),
      createNode('iterationStart:0', Operator.IterationStart, {
        parentId: 'iteration:0',
        type: 'iterationStartNode',
      }),
      createNode('message:0', Operator.Message, { parentId: 'iteration:0' }),
      createNode('message:1', Operator.Message, { parentId: 'iteration:0' }),
    ];
    const selectedEdge = createEdge('e1', 'message:0', 'message:1');
    const edges = [
      createEdge('e0', 'iterationStart:0', 'message:0'),
      selectedEdge,
    ];

    useGraphStore.setState({ nodes, edges });

    const { result } = renderHook(() => useBeforeDelete());
    let deletion: Awaited<ReturnType<typeof deleteFromHook>> | undefined;
    await act(async () => {
      deletion = await deleteFromHook(result.current.handleBeforeDelete, {
        nodes: [],
        edges: [selectedEdge],
      });
    });

    expect(deletion?.nodes).toEqual([]);
    expect(deletion?.edges.map((edge) => edge.id)).toEqual(['e1']);
  });

  it('does not delete edges of protected nodes when those nodes are rejected', async () => {
    const beginNode = createNode('begin', Operator.Begin);
    const messageNode = createNode('message:0', Operator.Message);
    const edge = createEdge('e1', 'begin', 'message:0');

    useGraphStore.setState({
      nodes: [beginNode, messageNode],
      edges: [edge],
    });

    const { result } = renderHook(() => useBeforeDelete());
    let deletion: Awaited<ReturnType<typeof deleteFromHook>> | undefined;
    await act(async () => {
      deletion = await deleteFromHook(result.current.handleBeforeDelete, {
        nodes: [beginNode],
        edges: [edge],
      });
    });

    expect(deletion?.nodes).toEqual([]);
    expect(deletion?.edges).toEqual([]);
  });

  it('deletes edges of a protected node when the other end is deleted', async () => {
    const beginNode = createNode('begin', Operator.Begin);
    const messageNode = createNode('message:0', Operator.Message);
    const edge = createEdge('e1', 'begin', 'message:0');

    useGraphStore.setState({
      nodes: [beginNode, messageNode],
      edges: [edge],
    });

    const { result } = renderHook(() => useBeforeDelete());
    let deletion: Awaited<ReturnType<typeof deleteFromHook>> | undefined;
    await act(async () => {
      deletion = await deleteFromHook(result.current.handleBeforeDelete, {
        nodes: [beginNode, messageNode],
        edges: [edge],
      });
    });

    expect(deletion?.nodes.map((node) => node.id)).toEqual(['message:0']);
    expect(deletion?.edges.map((edge) => edge.id)).toEqual(['e1']);
  });

  it('preserves agent downstream cleanup', async () => {
    const nodes = [
      createNode('agent:0', Operator.Agent),
      createNode('tool:0', Operator.Tool),
      createNode('message:0', Operator.Message),
    ];

    const edges = [
      createEdge('e1', 'agent:0', 'tool:0', {
        sourceHandle: NodeHandleId.AgentBottom,
      }),
      createEdge('e2', 'tool:0', 'message:0', {
        sourceHandle: NodeHandleId.Tool,
      }),
    ];

    useGraphStore.setState({ nodes, edges });

    const { result } = renderHook(() => useBeforeDelete());
    let deletion: Awaited<ReturnType<typeof deleteFromHook>> | undefined;
    await act(async () => {
      deletion = await deleteFromHook(result.current.handleBeforeDelete, {
        nodes: [nodes[0]],
        edges,
      });
    });

    expect(deletion?.nodes.map((node) => node.id).sort()).toEqual(
      ['agent:0', 'tool:0', 'message:0'].sort(),
    );
    expect(deletion?.edges.map((edge) => edge.id).sort()).toEqual(
      ['e1', 'e2'].sort(),
    );
  });

  it('expands iteration deletion to nested agent tool chains', async () => {
    const nodes = [
      createNode('iteration:0', Operator.Iteration, { type: 'group' }),
      createNode('iterationStart:0', Operator.IterationStart, {
        parentId: 'iteration:0',
        type: 'iterationStartNode',
      }),
      createNode('agent:0', Operator.Agent, { parentId: 'iteration:0' }),
      createNode('tool:0', Operator.Tool),
      createNode('message:0', Operator.Message),
      createNode('message:out', Operator.Message),
    ];

    const edges = [
      createEdge('e1', 'iterationStart:0', 'agent:0'),
      createEdge('e2', 'agent:0', 'tool:0', {
        sourceHandle: NodeHandleId.AgentBottom,
      }),
      createEdge('e3', 'tool:0', 'message:0', {
        sourceHandle: NodeHandleId.Tool,
      }),
      createEdge('e4', 'message:out', 'message:0'),
    ];

    useGraphStore.setState({ nodes, edges });

    const { result } = renderHook(() => useBeforeDelete());
    let deletion: Awaited<ReturnType<typeof deleteFromHook>> | undefined;
    await act(async () => {
      deletion = await deleteFromHook(result.current.handleBeforeDelete, {
        nodes: [nodes[0]],
        edges: [],
      });
    });

    expect(deletion?.nodes.map((node) => node.id).sort()).toEqual(
      [
        'iteration:0',
        'iterationStart:0',
        'agent:0',
        'tool:0',
        'message:0',
      ].sort(),
    );
    expect(deletion?.edges.map((edge) => edge.id).sort()).toEqual(
      ['e1', 'e2', 'e3', 'e4'].sort(),
    );
  });
});
