export type CrawlContent = {
  id: string;
  title: string;
  source_keyword: string;
};

export type StartCrawlTaskResponse = {
  task_id: string;
  status: string;
};

export type ListCrawlContentsResponse = {
  contents?: CrawlContent[];
};

export type UpdateKeywordsResponse = {
  keywords?: string[];
};

export type XhsRequestResult<T> = {
  data: T;
  method: "GET" | "POST" | "PUT";
  path: string;
  status: number;
};
