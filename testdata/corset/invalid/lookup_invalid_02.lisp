;;error:3:17-26:differing number of source and target columns (1 v 2)
(defcolumns (X :i16) (Y :i16) (one :i1 :constant 1))
(deflookup test ((one X)) ((one Y Y)))
