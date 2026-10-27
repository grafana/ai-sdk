import React from 'react';
import { Route, Routes } from 'react-router-dom';
import { AppRootProps } from '@grafana/data';

const Overview = React.lazy(() => import('../../pages/Overview'));

function App(props: AppRootProps) {
  return (
    <Routes>
      <Route path="*" element={<Overview />} />
    </Routes>
  );
}

export default App;
