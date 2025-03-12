import { createClient } from "next-sanity";
import  imageUrlBuilder  from '@sanity/image-url';

export const client = createClient({
    apiVersion: '2021-03-25',
    dataset: 'production',
    projectId: '18xoyd4u',
    useCdn: false,
})

const builder = imageUrlBuilder(client)

export function urlFor(source: any) {
    return builder.image(source);
}