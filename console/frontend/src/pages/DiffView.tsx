import React, { useEffect, useState } from 'react';
import {
  Title,
  Label,
  Split,
  SplitItem,
  Card,
  CardBody,
  CardTitle,
  TextInput,
  FormGroup,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  EmptyState,
  EmptyStateBody,
  EmptyStateHeader,
  EmptyStateIcon,
} from '@patternfly/react-core';
import { SearchIcon } from '@patternfly/react-icons';
import { safeFetch } from '../utils/api';

interface DiffEvent {
  id: string;
  resource: string;
  namespace: string;
  cluster: string;
  timestamp: string;
  action: string;
  agentName: string;
  policyName: string;
  before: Record<string, string>;
  after: Record<string, string>;
  status: string;
  dryRun: boolean;
  reason: string;
  aiResponse: string;
}

export const DiffView: React.FC = () => {
  const [events, setEvents] = useState<DiffEvent[]>([]);
  const [clusterFilter, setClusterFilter] = useState('');
  const [namespaceFilter, setNamespaceFilter] = useState('');
  const [timeRange, setTimeRange] = useState('all');

  const fetchDiffs = () => {
    const params = new URLSearchParams();
    if (clusterFilter) params.set('cluster', clusterFilter);
    if (namespaceFilter) params.set('namespace', namespaceFilter);
    const qs = params.toString();
    safeFetch<DiffEvent[]>(`/api/v1/diffs${qs ? '?' + qs : ''}`)
      .then(d => setEvents(d || []));
  };

  useEffect(() => {
    fetchDiffs();
  }, [clusterFilter, namespaceFilter]);

  const filteredEvents = events.filter(ev => {
    if (timeRange === 'all') return true;
    const hours = parseInt(timeRange, 10);
    if (isNaN(hours)) return true;
    const cutoff = new Date(Date.now() - hours * 3600000);
    return new Date(ev.timestamp) >= cutoff;
  });

  const actionColor = (action: string): 'green' | 'orange' | 'blue' | 'red' | 'grey' => {
    if (action.includes('ScaleUp') || action.includes('scale_up') || action.includes('Increase')) return 'orange';
    if (action.includes('ScaleDown') || action.includes('scale_down') || action.includes('Decrease')) return 'blue';
    if (action.includes('Add') || action.includes('Created')) return 'green';
    if (action.includes('Remove') || action.includes('Deleted')) return 'red';
    return 'grey';
  };

  const statusColor = (status: string): 'green' | 'blue' | 'red' | 'grey' => {
    switch (status) {
      case 'applied': return 'green';
      case 'dry-run': return 'blue';
      case 'rejected': return 'red';
      case 'pending-approval': return 'grey';
      default: return 'grey';
    }
  };

  const clusters = Array.from(new Set(events.map(e => e.cluster).filter(Boolean)));

  const renderDiffValue = (key: string, before: Record<string, string> | null, after: Record<string, string> | null) => {
    const bVal = before?.[key] ?? '';
    const aVal = after?.[key] ?? '';
    const changed = bVal !== aVal;
    return { bVal, aVal, changed };
  };

  const allKeys = (before: Record<string, string> | null, after: Record<string, string> | null): string[] => {
    const keys = new Set<string>();
    if (before) Object.keys(before).forEach(k => keys.add(k));
    if (after) Object.keys(after).forEach(k => keys.add(k));
    return Array.from(keys);
  };

  return (
    <>
      <Title headingLevel="h1" size="2xl" style={{ marginBottom: '1rem' }}>
        Diff View — Before / After Comparison
      </Title>
      <p style={{ marginBottom: '1.5rem', color: '#6a737d' }}>
        Side-by-side comparison of resource snapshots before and after corrections applied by Kairos agents.
      </p>

      <Toolbar>
        <ToolbarContent>
          <ToolbarItem>
            <FormGroup label="Cluster" fieldId="cluster-filter">
              <select
                id="cluster-filter"
                value={clusterFilter}
                onChange={e => setClusterFilter(e.target.value)}
                style={{ padding: '0.4rem 0.75rem', borderRadius: '4px', border: '1px solid #444', background: '#1a1a2e', color: '#e0e0e0' }}
              >
                <option value="">All clusters</option>
                {clusters.map(c => <option key={c} value={c}>{c}</option>)}
              </select>
            </FormGroup>
          </ToolbarItem>
          <ToolbarItem>
            <FormGroup label="Namespace" fieldId="namespace-filter">
              <TextInput
                id="namespace-filter"
                placeholder="Filter by namespace"
                value={namespaceFilter}
                onChange={(_e, val) => setNamespaceFilter(val)}
              />
            </FormGroup>
          </ToolbarItem>
          <ToolbarItem>
            <FormGroup label="Time range" fieldId="time-range">
              <select
                id="time-range"
                value={timeRange}
                onChange={e => setTimeRange(e.target.value)}
                style={{ padding: '0.4rem 0.75rem', borderRadius: '4px', border: '1px solid #444', background: '#1a1a2e', color: '#e0e0e0' }}
              >
                <option value="all">All time</option>
                <option value="1">Last 1 hour</option>
                <option value="6">Last 6 hours</option>
                <option value="24">Last 24 hours</option>
                <option value="72">Last 3 days</option>
                <option value="168">Last 7 days</option>
              </select>
            </FormGroup>
          </ToolbarItem>
        </ToolbarContent>
      </Toolbar>

      {filteredEvents.length === 0 ? (
        <Card isFlat style={{ marginTop: '1rem' }}>
          <CardBody>
            <EmptyState>
              <EmptyStateHeader
                titleText="No correction events recorded yet"
                headingLevel="h4"
                icon={<EmptyStateIcon icon={SearchIcon} />}
              />
              <EmptyStateBody>
                When Kairos agents apply resource corrections, before/after diffs will appear here.
                Ensure KairosAgent CRs are deployed and actively monitoring workloads.
              </EmptyStateBody>
            </EmptyState>
          </CardBody>
        </Card>
      ) : (
        filteredEvents.map((event) => (
          <Card isFlat key={event.id || `${event.resource}-${event.timestamp}`} style={{ marginBottom: '1.5rem', marginTop: '1rem' }}>
            <CardTitle>
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', flexWrap: 'wrap' }}>
                <strong>{event.resource}</strong>
                <Label color="blue">{event.namespace}</Label>
                <Label color="cyan">{event.cluster}</Label>
                <Label color={actionColor(event.action)}>{event.action}</Label>
                <Label color={statusColor(event.status)}>{event.status}</Label>
                {event.dryRun && <Label color="gold">dry-run</Label>}
                <span style={{ color: '#8b949e', fontSize: '0.85rem' }}>
                  {new Date(event.timestamp).toLocaleString()}
                </span>
                {event.agentName && (
                  <Label color="purple">{event.agentName}</Label>
                )}
              </div>
            </CardTitle>
            <CardBody>
              {event.reason && (
                <p style={{ marginBottom: '1rem', color: '#8b949e', fontStyle: 'italic' }}>
                  {event.reason}
                </p>
              )}
              <Split hasGutter>
                <SplitItem isFilled>
                  <Card isFlat isCompact>
                    <CardTitle>
                      <Label color="red">Before</Label>
                    </CardTitle>
                    <CardBody>
                      {event.before && Object.keys(event.before).length > 0 ? (
                        <div style={{ fontFamily: 'monospace', fontSize: '0.9rem' }}>
                          {allKeys(event.before, event.after).map(key => {
                            const { bVal, changed } = renderDiffValue(key, event.before, event.after);
                            return (
                              <div
                                key={key}
                                style={{
                                  padding: '0.25rem 0.5rem',
                                  background: changed ? 'rgba(248, 81, 73, 0.15)' : 'transparent',
                                  borderLeft: changed ? '3px solid #f85149' : '3px solid transparent',
                                  marginBottom: '2px',
                                }}
                              >
                                <span style={{ color: '#8b949e' }}>{key}: </span>
                                <span style={{ color: changed ? '#f85149' : '#e0e0e0' }}>{bVal || '(none)'}</span>
                              </div>
                            );
                          })}
                        </div>
                      ) : (
                        <span style={{ color: '#6a737d' }}>(no previous state)</span>
                      )}
                    </CardBody>
                  </Card>
                </SplitItem>
                <SplitItem isFilled>
                  <Card isFlat isCompact>
                    <CardTitle>
                      <Label color="green">After</Label>
                    </CardTitle>
                    <CardBody>
                      {event.after && Object.keys(event.after).length > 0 ? (
                        <div style={{ fontFamily: 'monospace', fontSize: '0.9rem' }}>
                          {allKeys(event.before, event.after).map(key => {
                            const { aVal, changed } = renderDiffValue(key, event.before, event.after);
                            return (
                              <div
                                key={key}
                                style={{
                                  padding: '0.25rem 0.5rem',
                                  background: changed ? 'rgba(63, 185, 80, 0.15)' : 'transparent',
                                  borderLeft: changed ? '3px solid #3fb950' : '3px solid transparent',
                                  marginBottom: '2px',
                                }}
                              >
                                <span style={{ color: '#8b949e' }}>{key}: </span>
                                <span style={{ color: changed ? '#3fb950' : '#e0e0e0' }}>{aVal || '(none)'}</span>
                              </div>
                            );
                          })}
                        </div>
                      ) : (
                        <span style={{ color: '#6a737d' }}>(no new state)</span>
                      )}
                    </CardBody>
                  </Card>
                </SplitItem>
              </Split>
              {event.aiResponse && (
                <div style={{ marginTop: '1rem', padding: '0.75rem', background: 'rgba(56, 132, 244, 0.08)', borderRadius: '4px', borderLeft: '3px solid #3884f4' }}>
                  <strong style={{ color: '#3884f4' }}>AI Analysis:</strong>
                  <p style={{ margin: '0.25rem 0 0', color: '#c9d1d9' }}>{event.aiResponse}</p>
                </div>
              )}
            </CardBody>
          </Card>
        ))
      )}
    </>
  );
};
