// client.js
import { ApolloClient, InMemoryCache, gql } from '@apollo/client';


const hygraphclient = new ApolloClient({
  uri: 'https://api-us-east-1-shared-usea1-02.hygraph.com/v2/clqtyw1icggnc01ui7yon7108/master',
  cache: new InMemoryCache(),
});

const getPosts = gql`
      query getPosts {
          assets {
              createdAt
              id
              publishedAt
              fileName
              url
              updatedAt
          }
          postsConnection {
              edges {
                  node {
                      author {
                          bio
                          name
                          id
                          photo {
                              url
                          }
                      }
                      createdAt
                      slug
                      title
                      excerpt
                      featuredImage {
                          url
                      }
                      categories {
                          name
                          slug
                      }
                  }
              }
          }
      }
  `;

  export async function getPost() {
    const { data } = await hygraphclient.query({
      query: getPosts,
    });
    return data.postConnection.edges;
  }