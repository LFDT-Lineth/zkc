;;error:3:17-28:differing number of source and target columns (2 v 1)
(defcolumns (X :i16) (Y :i16) (one :i1 :constant 1))
(deflookup test ((one X X)) ((one Y)))
