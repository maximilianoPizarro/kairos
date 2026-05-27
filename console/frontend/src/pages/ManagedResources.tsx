import React, { useEffect, useState } from 'react';
import {
  Title,
  Label,
  Pagination,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';

interface ManagedResource {
  name: string;
  namespace: string;
  kind: string;
  cluster: string;
  policy: string;
  agent: string;
  currentCPU: string;
  currentMemory: string;
  status: string;
}

export const ManagedResources: React.FC = () => {
  const [resources, setResources] = useState<ManagedResource[]>([]);
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(10);

  useEffect(() => {
    fetch('/api/v1/managed-resources').then(r => r.json()).then(setResources);
  }, []);

  const startIdx = (page - 1) * perPage;
  const paginatedResources = resources.slice(startIdx, startIdx + perPage);

  return (
    <>
      <Title headingLevel="h1" size="2xl" style={{ marginBottom: '1rem' }}>
        Managed Resources
      </Title>
      <p style={{ marginBottom: '1rem', color: '#6a737d' }}>
        Resources annotated with <code>kairos.io/managed: "true"</code> across all clusters.
        These are actively managed by Kairos agents via Server-Side Apply.
      </p>

      <Pagination
        itemCount={resources.length}
        perPage={perPage}
        page={page}
        onSetPage={(_e, p) => setPage(p)}
        onPerPageSelect={(_e, pp) => { setPerPage(pp); setPage(1); }}
        style={{ marginBottom: '1rem' }}
      />

      <Table aria-label="Managed Resources">
        <Thead>
          <Tr>
            <Th>Name</Th>
            <Th>Namespace</Th>
            <Th>Kind</Th>
            <Th>Cluster</Th>
            <Th>Policy</Th>
            <Th>Agent</Th>
            <Th>CPU</Th>
            <Th>Memory</Th>
            <Th>Status</Th>
          </Tr>
        </Thead>
        <Tbody>
          {paginatedResources.map((res, idx) => (
            <Tr key={idx}>
              <Td>{res.name}</Td>
              <Td>{res.namespace}</Td>
              <Td><Label color="blue">{res.kind}</Label></Td>
              <Td><Label color="purple">{res.cluster}</Label></Td>
              <Td>{res.policy || '-'}</Td>
              <Td>{res.agent || '-'}</Td>
              <Td><code>{res.currentCPU || '-'}</code></Td>
              <Td><code>{res.currentMemory || '-'}</code></Td>
              <Td><Label color="green">{res.status}</Label></Td>
            </Tr>
          ))}
        </Tbody>
      </Table>

      {resources.length > perPage && (
        <Pagination
          itemCount={resources.length}
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
