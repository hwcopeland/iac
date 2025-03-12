import { gql } from "@apollo/client";

export const GET_REPOSITORY = gql`
  query getRepository($username: String!, $repository: String!) {
    repository(name: $repository, owner: $username) {
      isFork
      isTemplate
      isArchived
      url
      forks {
        totalCount
      }
      stargazers {
        totalCount
      }
      name
      description
    }
  }
`;