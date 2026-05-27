import React, { useEffect, useState } from 'react';
import {
  Title,
  Label,
  Pagination,
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
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(10);

  useEffect(() => {
    fetch('/api/v1/events').then(r => r.json()).then(setEvents);
  }, []);

  const actionColor = (action: string) => {
    if (action.includes('Increase') || action.includes('Optimized') || action.includes('ScaleUp')) return 'orange';
    if (action.includes('Decrease') || action.includes('ScaleDown')) return 'blue';
    if (action.includes('Add') || action.includes('Created')) return 'green';
    return 'grey';
  };

  const startIdx = (page - 1) * perPage;
  const paginatedEvents = events.slice(startIdx, startIdx + perPage);

  return (
    <>
      <Title headingLevel="h1" size="2xl" style={{ marginBottom: '1rem' }}>
        Scaling Events
      </Title>

      <Pagination
        itemCount={events.length}
        perPage={perPage}
        page={page}
        onSetPage={(_e, p) => setPage(p)}
        onPerPageSelect={(_e, pp) => { setPerPage(pp); setPage(1); }}
        style={{ marginBottom: '1rem' }}
      />

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
          {paginatedEvents.map((event, idx) => (
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

      {events.length > perPage && (
        <Pagination
          itemCount={events.length}
          perPage={perPage}
          page={page}
          onSetPage={(_e, p) => setPage(p)}
          onPerPageSelect={(_e, pp) => { setPerPage(pp); setPage(1); }}
          variant="bottom"
          style={{ marginTop: '1rem' }}
        />
      )}
    </>
  );
};
