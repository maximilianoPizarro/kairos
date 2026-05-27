import React, { useState, useEffect } from 'react';
import {
  Page,
  Masthead,
  MastheadMain,
  MastheadBrand,
  MastheadContent,
  PageSidebar,
  PageSidebarBody,
  PageSection,
  Nav,
  NavList,
  NavItem,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  ToolbarGroup,
  Label,
  Avatar,
  Dropdown,
  DropdownItem,
  DropdownList,
  MenuToggle,
} from '@patternfly/react-core';
import { Dashboard } from './pages/Dashboard';
import { Agents } from './pages/Agents';
import { Policies } from './pages/Policies';
import { Events } from './pages/Events';
import { Observability } from './pages/Observability';
import { ManagedResources } from './pages/ManagedResources';
import { Approvals } from './pages/Approvals';
import { History } from './pages/History';
import { DiffView } from './pages/DiffView';

type PageKey = 'dashboard' | 'agents' | 'policies' | 'events' | 'observability' | 'resources' | 'approvals' | 'history' | 'diffview';

interface UserInfo {
  username: string;
  authenticated: boolean;
}

export const App: React.FC = () => {
  const [activePage, setActivePage] = useState<PageKey>('dashboard');
  const [userInfo, setUserInfo] = useState<UserInfo>({ username: 'anonymous', authenticated: false });
  const [userMenuOpen, setUserMenuOpen] = useState(false);

  useEffect(() => {
    fetch('/api/v1/user')
      .then(r => r.json())
      .then((data: UserInfo) => setUserInfo(data))
      .catch(() => setUserInfo({ username: 'anonymous', authenticated: false }));
  }, []);

  const handleLogout = () => {
    window.location.href = '/oauth/sign_out';
  };

  const renderPage = () => {
    switch (activePage) {
      case 'dashboard': return <Dashboard />;
      case 'agents': return <Agents />;
      case 'policies': return <Policies />;
      case 'events': return <Events />;
      case 'observability': return <Observability />;
      case 'resources': return <ManagedResources />;
      case 'approvals': return <Approvals />;
      case 'history': return <History />;
      case 'diffview': return <DiffView />;
      default: return <Dashboard />;
    }
  };

  const userDropdownItems = (
    <DropdownList>
      <DropdownItem key="user-info" isDisabled>
        {userInfo.authenticated ? `Logged in as ${userInfo.username}` : 'Not authenticated'}
      </DropdownItem>
      {userInfo.authenticated && (
        <DropdownItem key="logout" onClick={handleLogout}>
          Log out
        </DropdownItem>
      )}
    </DropdownList>
  );

  const header = (
    <Masthead>
      <MastheadMain>
        <MastheadBrand>
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48" width="36" height="36">
              <defs>
                <linearGradient id="logo-g1" x1="0%" y1="0%" x2="100%" y2="100%">
                  <stop offset="0%" stopColor="#00bcd4"/>
                  <stop offset="100%" stopColor="#1a237e"/>
                </linearGradient>
                <linearGradient id="logo-g2" x1="0%" y1="0%" x2="0%" y2="100%">
                  <stop offset="0%" stopColor="#00e5ff"/>
                  <stop offset="100%" stopColor="#0091ea"/>
                </linearGradient>
              </defs>
              <circle cx="24" cy="24" r="23" fill="#0d1117"/>
              <circle cx="24" cy="24" r="22" fill="none" stroke="url(#logo-g2)" strokeWidth="0.8" opacity="0.6"/>
              <path d="M17 10 L31 10 L25.5 22.5 L24 24 L22.5 22.5 L17 10 Z" fill="url(#logo-g1)" opacity="0.9"/>
              <path d="M17 38 L31 38 L25.5 25.5 L24 24 L22.5 25.5 L17 38 Z" fill="url(#logo-g1)" opacity="0.9"/>
              <path d="M16 9 L32 9" stroke="#00e5ff" strokeWidth="1.5" strokeLinecap="round"/>
              <path d="M16 39 L32 39" stroke="#00e5ff" strokeWidth="1.5" strokeLinecap="round"/>
              <path d="M12 20 C9 18 5 17 3 18 C5 20 8 22 12 23" fill="#00bcd4" opacity="0.7"/>
              <path d="M36 20 C39 18 43 17 45 18 C43 20 40 22 36 23" fill="#00bcd4" opacity="0.7"/>
              <circle cx="24" cy="24" r="2.5" fill="#00e5ff"/>
              <circle cx="24" cy="24" r="1.2" fill="#0d1117"/>
              <circle cx="24" cy="15" r="0.6" fill="#ffab40" opacity="0.8"/>
              <circle cx="24" cy="32" r="0.5" fill="#ffab40" opacity="0.6"/>
            </svg>
            <Title headingLevel="h1" size="xl" style={{ color: '#00e5ff' }}>Kairos</Title>
          </div>
        </MastheadBrand>
      </MastheadMain>
      <MastheadContent>
        <Toolbar isFullHeight>
          <ToolbarContent>
            <ToolbarGroup align={{ default: 'alignLeft' }}>
              <ToolbarItem>
                <Label color="green">Operator v2.0.0</Label>
              </ToolbarItem>
              <ToolbarItem>
                <Label color="blue">3 Clusters</Label>
              </ToolbarItem>
            </ToolbarGroup>
            <ToolbarGroup align={{ default: 'alignRight' }}>
              <ToolbarItem>
                <Dropdown
                  isOpen={userMenuOpen}
                  onSelect={() => setUserMenuOpen(false)}
                  onOpenChange={(open) => setUserMenuOpen(open)}
                  toggle={(toggleRef) => (
                    <MenuToggle
                      ref={toggleRef}
                      onClick={() => setUserMenuOpen(!userMenuOpen)}
                      isExpanded={userMenuOpen}
                      variant="plain"
                    >
                      <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                        <Avatar
                          src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 36 36'%3E%3Ccircle cx='18' cy='18' r='18' fill='%23161b22'/%3E%3Ccircle cx='18' cy='14' r='6' fill='%2300e5ff'/%3E%3Cpath d='M6 36 Q6 26 18 26 Q30 26 30 36' fill='%2300e5ff'/%3E%3C/svg%3E"
                          alt="User avatar"
                          size="sm"
                        />
                        <span style={{ color: userInfo.authenticated ? '#00e676' : '#8b949e' }}>
                          {userInfo.username}
                        </span>
                      </div>
                    </MenuToggle>
                  )}
                  popperProps={{ position: 'right' }}
                >
                  {userDropdownItems}
                </Dropdown>
              </ToolbarItem>
            </ToolbarGroup>
          </ToolbarContent>
        </Toolbar>
      </MastheadContent>
    </Masthead>
  );

  const sidebar = (
    <PageSidebar>
      <PageSidebarBody>
        <Nav>
          <NavList>
            <NavItem isActive={activePage === 'dashboard'} onClick={() => setActivePage('dashboard')}>
              Dashboard
            </NavItem>
            <NavItem isActive={activePage === 'agents'} onClick={() => setActivePage('agents')}>
              AI Agents
            </NavItem>
            <NavItem isActive={activePage === 'policies'} onClick={() => setActivePage('policies')}>
              Scaling Policies
            </NavItem>
            <NavItem isActive={activePage === 'events'} onClick={() => setActivePage('events')}>
              Events
            </NavItem>
            <NavItem isActive={activePage === 'observability'} onClick={() => setActivePage('observability')}>
              Observability
            </NavItem>
            <NavItem isActive={activePage === 'resources'} onClick={() => setActivePage('resources')}>
              Managed Resources
            </NavItem>
            <NavItem isActive={activePage === 'approvals'} onClick={() => setActivePage('approvals')}>
              Approvals
            </NavItem>
            <NavItem isActive={activePage === 'history'} onClick={() => setActivePage('history')}>
              History
            </NavItem>
            <NavItem isActive={activePage === 'diffview'} onClick={() => setActivePage('diffview')}>
              Diff View
            </NavItem>
          </NavList>
        </Nav>
      </PageSidebarBody>
    </PageSidebar>
  );

  return (
    <Page header={header} sidebar={sidebar}>
      <PageSection>
        {renderPage()}
      </PageSection>
    </Page>
  );
};
