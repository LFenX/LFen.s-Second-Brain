/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { Button } from '@/components/ui/button';
import { Panel } from '@/components/form-fields';
import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import { useTranslation } from 'react-i18next';

import { SearchInput } from '@/components/ui/input';
import { UserPlus } from 'lucide-react';
import { useState } from 'react';
import { ProfileSettingWrapperCard } from '../components/user-setting-header';
import AddingUserModal from './add-user-modal';
import { useAddUser } from './hooks';
import TenantTable from './tenant-table';
import KnowledgeDepartments from './knowledge-departments';
import UserTable from './user-table';

const UserSettingTeam = () => {
  const { data: userInfo } = useFetchUserInfo();
  const { t } = useTranslation();
  const [searchTerm, setSearchTerm] = useState('');
  const [searchUser, setSearchUser] = useState('');
  const {
    addingTenantModalVisible,
    hideAddingTenantModal,
    showAddingTenantModal,
    handleAddUserOk,
  } = useAddUser();

  return (
    <ProfileSettingWrapperCard
      header={
        <header>
          <h2 className="text-2xl font-medium text-text-primary">
            {userInfo?.nickname + ' ' + t('setting.workspace')}
          </h2>
        </header>
      }
    >
      <div className="h-full space-y-5 overflow-y-auto overflow-x-hidden p-5">
        <Panel
          title={t('setting.teamMembers')}
          actions={
            <>
              <SearchInput
                className="w-40"
                placeholder={t('common.search')}
                value={searchUser}
                onChange={(e) => setSearchUser(e.target.value)}
              />
              <Button onClick={showAddingTenantModal}>
                <UserPlus className="size-4" />
                {t('setting.invite')}
              </Button>
            </>
          }
        >
          <UserTable searchUser={searchUser}></UserTable>
        </Panel>

        <Panel
          title={t('setting.joinedTeams')}
          actions={
            <SearchInput
              className="w-40"
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              placeholder={t('common.search')}
            />
          }
        >
          <TenantTable searchTerm={searchTerm}></TenantTable>
        </Panel>
        <KnowledgeDepartments />
      </div>

      {addingTenantModalVisible && (
        <AddingUserModal
          visible
          hideModal={hideAddingTenantModal}
          onOk={handleAddUserOk}
        ></AddingUserModal>
      )}
    </ProfileSettingWrapperCard>
  );
};

export default UserSettingTeam;
