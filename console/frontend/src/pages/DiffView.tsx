import React, { useEffect, useState } from 'react';
import {
  Title,
  Label,
  Split,
  SplitItem,
  Card,
  CardBody,
  CardTitle,
  CodeBlock,
  CodeBlockCode,
} from '@patternfly/react-core';

interface DiffEvent {
  id: string;
  resource: string;
  namespace: string;
  cluster: string;
  timestamp: string;
  action: string;
  before: Record<string, unknown>;
  after: Record<string, unknown>;
}

export const DiffView: React.FC = () => {
  const [events, setEvents] = useState<DiffEvent[]>([]);

  useEffect(() => {
    fetch('/api/v1/events')
      .then(r => r.json())
      .then(data => setEvents(data || []))
      .catch(() => setEvents([]));
  }, []);

  const actionColor = (action: string): 'green' | 'orange' | 'blue' | 'red' | 'grey' => {
    if (action.includes('ScaleUp') || action.includes('Increase')) return 'orange';
    if (action.includes('ScaleDown') || action.includes('Decrease')) return 'blue';
    if (action.includes('Add') || action.includes('Created')) return 'green';
    if (action.includes('Remove') || action.includes('Deleted')) return 'red';
    return 'grey';
  };

  return (
    <>
      <Title headingLevel="h1" size="2xl" style={{ marginBottom: '1rem' }}>
        Diff View — Before / After Comparison
      </Title>
      <p style={{ marginBottom: '1.5rem', color: '#6a737d' }}>
        Side-by-side comparison of resource snapshots before and after corrections applied by Kairos agents.
      </p>

      {events.length === 0 ? (
        <Card isFlat>
          <CardBody style={{ textAlign: 'center', padding: '2rem' }}>
            No diff events available.
          </CardBody>
        </Card>
      ) : (
        events.map((event) => (
          <Card isFlat key={event.id || `${event.resource}-${event.timestamp}`} style={{ marginBottom: '1.5rem' }}>
            <CardTitle>
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', flexWrap: 'wrap' }}>
                <strong>{event.resource}</strong>
                <Label color="blue">{event.namespace}</Label>
                <Label color="cyan">{event.cluster}</Label>
                <Label color={actionColor(event.action)}>{event.action}</Label>
                <span style={{ color: '#8b949e', fontSize: '0.85rem' }}>
                  {new Date(event.timestamp).toLocaleString()}
                </span>
              </div>
            </CardTitle>
            <CardBody>
              <Split hasGutter>
                <SplitItem isFilled>
                  <Card isFlat isCompact>
                    <CardTitle>
                      <Label color="red">Before</Label>
                    </CardTitle>
                    <CardBody>
                      <CodeBlock>
                        <CodeBlockCode>
                          {event.before
                            ? JSON.stringify(event.before, null, 2)
                            : '(no previous state)'}
                        </CodeBlockCode>
                      </CodeBlock>
                    </CardBody>
                  </Card>
                </SplitItem>
                <SplitItem isFilled>
                  <Card isFlat isCompact>
                    <CardTitle>
                      <Label color="green">After</Label>
                    </CardTitle>
                    <CardBody>
                      <CodeBlock>
                        <CodeBlockCode>
                          {event.after
                            ? JSON.stringify(event.after, null, 2)
                            : '(no new state)'}
                        </CodeBlockCode>
                      </CodeBlock>
                    </CardBody>
                  </Card>
                </SplitItem>
              </Split>
            </CardBody>
          </Card>
        ))
      )}
    </>
  );
};
