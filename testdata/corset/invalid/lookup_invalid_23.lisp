;;error:3:17-24:differing number of source and target columns (1 v 2)
(defcolumns (X :i16) (Y :i16))
(deflookup test ((_ X)) ((_ Y Y)))
