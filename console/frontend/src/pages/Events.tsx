import React, { useEffect, useState } from 'react';
import {
  Title,
  Label,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';

interface EventInfo {
  timestamp: string;
  type: string;
  resource: string;
  namespace: string;
  action: string;
  detail: string;
  cluster: string;
}

export const Events: React.FC = () => {
  const [events, setEvents] = useState<EventInfo[]>([]);

  useEffect(() => {
    fetch('/api/v1/events').then(r => r.json()).then(setEvents);
  }, []);

  const actionColor = (action: string) => {
    if (action.includes('Increase')) return 'orange';
    if (action.includes('Decrease')) return 'blue';
    if (action.includes('Add')) return 'green';
    return 'grey';
  };

  return (
    <>
      <Title headingLevel="h1" size="2xl" style={{ marginBottom: '1rem' }}>
        Scaling Events
      </Title>

      <Table aria-label="Events table">
        <Thead>
          <Tr>
            <Th>Time</Th>
            <Th>Cluster</Th>
            <Th>Resource</Th>
            <Th>Namespace</Th>
            <Th>Action</Th>
            <Th>Detail</Th>
          </Tr>
        </Thead>
        <Tbody>
          {events.map((event, idx) => (
            <Tr key={idx}>
              <Td>{new Date(event.timestamp).toLocaleString()}</Td>
              <Td><Label>{event.cluster}</Label></Td>
              <Td>{event.resource}</Td>
              <Td>{event.namespace}</Td>
              <Td><Label color={actionColor(event.action)}>{event.action}</Label></Td>
              <Td>{event.detail}</Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
    </>
  );
};
