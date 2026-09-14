export type OrganizationMembership = {
  id: string;
  roles?: string[];
};

export type SocialContent = {
  id: string;
  title: string;
  source_keyword: string;
  platform: string;
};

export type SocialRequestResult<T> = {
  data: T;
  method: "GET";
  path: string;
  status: number;
};

export type ListOrganizationsResponse = {
  organizations?: OrganizationMembership[];
};

export type SearchContentsResponse = {
  contents?: SocialContent[];
};
